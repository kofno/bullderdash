// Package store provides an embedded SQLite-backed history of BullMQ jobs.
//
// The event collector feeds terminal (and in-flight) job records into an
// asynchronous, buffered writer so the live queue and the collector loop are
// never blocked on disk I/O. Search and detail reads are served from this
// local database (with an FTS5 full-text index over payloads) instead of
// scanning live Redis.
//
// The driver is modernc.org/sqlite (pure Go, no CGO) so bull-der-dash keeps
// building as a single static binary.
package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/kofno/bullderdash/internal/metrics"
	_ "modernc.org/sqlite"
)

// Record is a single job observation handed to the writer. Zero-valued
// optional fields (empty strings, zero timestamps) are treated as "unknown"
// and will not overwrite previously persisted non-zero values on upsert.
type Record struct {
	ID             string
	Queue          string
	Name           string
	State          string // Completed, Failed, Active, Waiting, Delayed
	Attempts       int
	ExecutionDepth int
	TraceID        string
	CreatedAtMs    int64
	FinishedAtMs   int64
	LastError      string
	Data           string // JSON payload
	Opts           string // JSON options
}

// SearchRow mirrors the shape consumed by the console UI (/v1/search).
type SearchRow struct {
	ID             string `json:"id"`
	Name           string `json:"name"`
	State          string `json:"state"`
	Attempts       int    `json:"attempts"`
	ExecutionDepth int    `json:"execution_depth"`
	TraceID        string `json:"trace_id"`
	CreatedAt      int64  `json:"created_at"`
	FinishedAt     int64  `json:"finished_at"`
	LastError      string `json:"last_error"`
	Queue          string `json:"queue"`
}

// JobDetail is the single-job drill-down (/v1/jobs/{id}).
type JobDetail struct {
	SearchRow
	Data string `json:"data"`
	Opts string `json:"opts"`
}

// SearchParams are the supported query predicates. An empty struct (no
// predicate) is rejected by the caller, matching the console contract.
type SearchParams struct {
	Query   string // free text (FTS over id/name/trace_id/last_error/data)
	Name    string // exact job name
	State   string // exact state
	TraceID string // exact trace id (lineage drill-down)
	SinceMs int64  // finished_at >= SinceMs when > 0
	Limit   int
}

// Retention controls the sweeper. A zero duration disables that tier; a zero
// MaxRows disables the hard cap.
type Retention struct {
	CompletedTTL time.Duration
	FailedTTL    time.Duration
	MaxRows      int64
}

// Config configures the store and its async writer.
type Config struct {
	Path          string
	WriteBuffer   int           // channel capacity for pending records
	BatchSize     int           // max records flushed per transaction
	FlushInterval time.Duration // max delay before a partial batch flushes
}

// Store owns the database handle and the background writer goroutine.
type Store struct {
	db   *sql.DB
	cfg  Config
	recs chan Record
	done chan struct{}
}

const defaultLimit = 100
const maxLimit = 1000

// New opens (creating if needed) the SQLite database, applies migrations and
// returns a ready Store. Call Run to start the async writer.
func New(cfg Config) (*Store, error) {
	cfg = normalize(cfg)

	dsn := cfg.Path +
		"?_pragma=busy_timeout(5000)" +
		"&_pragma=journal_mode(WAL)" +
		"&_pragma=synchronous(NORMAL)" +
		"&_pragma=foreign_keys(ON)"

	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	// Serialize writers at the pool level; WAL still allows concurrent readers
	// via separate connections opened on demand.
	db.SetMaxOpenConns(8)
	db.SetConnMaxIdleTime(5 * time.Minute)

	if err := migrate(db); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("migrate: %w", err)
	}

	return &Store{
		db:   db,
		cfg:  cfg,
		recs: make(chan Record, cfg.WriteBuffer),
		done: make(chan struct{}),
	}, nil
}

func normalize(cfg Config) Config {
	if cfg.Path == "" {
		cfg.Path = "bullderdash.db"
	}
	if cfg.WriteBuffer <= 0 {
		cfg.WriteBuffer = 4096
	}
	if cfg.BatchSize <= 0 {
		cfg.BatchSize = 256
	}
	if cfg.FlushInterval <= 0 {
		cfg.FlushInterval = time.Second
	}
	return cfg
}

func migrate(db *sql.DB) error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS job_history (
			rowid           INTEGER PRIMARY KEY AUTOINCREMENT,
			id              TEXT NOT NULL,
			queue           TEXT NOT NULL,
			name            TEXT,
			state           TEXT,
			attempts        INTEGER NOT NULL DEFAULT 0,
			execution_depth INTEGER NOT NULL DEFAULT 0,
			trace_id        TEXT,
			created_at      INTEGER NOT NULL DEFAULT 0,
			finished_at     INTEGER NOT NULL DEFAULT 0,
			last_error      TEXT,
			data            TEXT,
			opts            TEXT,
			updated_at      INTEGER NOT NULL DEFAULT 0,
			UNIQUE(queue, id)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_job_history_trace ON job_history(trace_id)`,
		`CREATE INDEX IF NOT EXISTS idx_job_history_state ON job_history(state)`,
		`CREATE INDEX IF NOT EXISTS idx_job_history_name ON job_history(name)`,
		`CREATE INDEX IF NOT EXISTS idx_job_history_finished ON job_history(finished_at)`,
		`CREATE INDEX IF NOT EXISTS idx_job_history_sweep ON job_history(state, finished_at, created_at)`,
		`CREATE VIRTUAL TABLE IF NOT EXISTS job_fts USING fts5(
			id, name, trace_id, last_error, data,
			content='job_history', content_rowid='rowid',
			tokenize='unicode61'
		)`,
		`CREATE TRIGGER IF NOT EXISTS job_history_ai AFTER INSERT ON job_history BEGIN
			INSERT INTO job_fts(rowid, id, name, trace_id, last_error, data)
			VALUES (new.rowid, new.id, new.name, new.trace_id, new.last_error, new.data);
		END`,
		`CREATE TRIGGER IF NOT EXISTS job_history_ad AFTER DELETE ON job_history BEGIN
			INSERT INTO job_fts(job_fts, rowid, id, name, trace_id, last_error, data)
			VALUES ('delete', old.rowid, old.id, old.name, old.trace_id, old.last_error, old.data);
		END`,
		`CREATE TRIGGER IF NOT EXISTS job_history_au AFTER UPDATE ON job_history BEGIN
			INSERT INTO job_fts(job_fts, rowid, id, name, trace_id, last_error, data)
			VALUES ('delete', old.rowid, old.id, old.name, old.trace_id, old.last_error, old.data);
			INSERT INTO job_fts(rowid, id, name, trace_id, last_error, data)
			VALUES (new.rowid, new.id, new.name, new.trace_id, new.last_error, new.data);
		END`,
	}
	for _, s := range stmts {
		if _, err := db.Exec(s); err != nil {
			return fmt.Errorf("exec migration: %w\n%s", err, s)
		}
	}
	return nil
}

// Enqueue hands a record to the async writer. It never blocks the caller: if
// the buffer is full the record is dropped and counted, protecting the event
// collector loop from disk stalls.
func (s *Store) Enqueue(rec Record) {
	normalizeRecord(&rec)
	select {
	case s.recs <- rec:
	default:
		metrics.StoreWritesDropped.Inc()
	}
}

func normalizeRecord(rec *Record) {
	rec.State = titleState(rec.State)
	if rec.Attempts < 0 {
		rec.Attempts = 0
	}
	if rec.ExecutionDepth < 0 {
		rec.ExecutionDepth = 0
	}
}

// titleState normalizes a BullMQ state/event token to the capitalized form the
// console renders (Completed, Failed, Active, Waiting, Delayed).
func titleState(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	return strings.ToUpper(s[:1]) + strings.ToLower(s[1:])
}

// Run drives the async writer until ctx is cancelled, flushing any buffered
// records on shutdown. Intended to run in its own goroutine.
func (s *Store) Run(ctx context.Context) {
	defer close(s.done)

	ticker := time.NewTicker(s.cfg.FlushInterval)
	defer ticker.Stop()

	batch := make([]Record, 0, s.cfg.BatchSize)

	flush := func() {
		if len(batch) == 0 {
			return
		}
		if err := s.writeBatch(ctx, batch); err != nil && ctx.Err() == nil {
			metrics.StoreWriteErrors.Inc()
		}
		batch = batch[:0]
	}

	for {
		select {
		case <-ctx.Done():
			// Best-effort drain of whatever is already buffered.
			for {
				select {
				case rec := <-s.recs:
					batch = append(batch, rec)
					if len(batch) >= s.cfg.BatchSize {
						s.flushDetached(batch)
						batch = batch[:0]
					}
				default:
					s.flushDetached(batch)
					return
				}
			}
		case rec := <-s.recs:
			batch = append(batch, rec)
			if len(batch) >= s.cfg.BatchSize {
				flush()
			}
		case <-ticker.C:
			flush()
		}
	}
}

// flushDetached writes a final batch during shutdown using a fresh, short
// timeout context so a cancelled parent context does not abort the drain.
func (s *Store) flushDetached(batch []Record) {
	if len(batch) == 0 {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := s.writeBatch(ctx, batch); err != nil {
		metrics.StoreWriteErrors.Inc()
	}
}

const upsertSQL = `
INSERT INTO job_history
	(id, queue, name, state, attempts, execution_depth, trace_id,
	 created_at, finished_at, last_error, data, opts, updated_at)
VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?)
ON CONFLICT(queue, id) DO UPDATE SET
	name            = excluded.name,
	state           = excluded.state,
	attempts        = MAX(excluded.attempts, job_history.attempts),
	execution_depth = CASE WHEN excluded.execution_depth > 0 THEN excluded.execution_depth ELSE job_history.execution_depth END,
	trace_id        = COALESCE(NULLIF(excluded.trace_id, ''), job_history.trace_id),
	created_at      = CASE WHEN excluded.created_at > 0 THEN excluded.created_at ELSE job_history.created_at END,
	finished_at     = CASE WHEN excluded.finished_at > 0 THEN excluded.finished_at ELSE job_history.finished_at END,
	last_error      = COALESCE(NULLIF(excluded.last_error, ''), job_history.last_error),
	data            = COALESCE(NULLIF(excluded.data, ''), job_history.data),
	opts            = COALESCE(NULLIF(excluded.opts, ''), job_history.opts),
	updated_at      = excluded.updated_at
`

func (s *Store) writeBatch(ctx context.Context, batch []Record) error {
	start := time.Now()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	stmt, err := tx.PrepareContext(ctx, upsertSQL)
	if err != nil {
		_ = tx.Rollback()
		return err
	}
	defer stmt.Close()

	now := time.Now().UnixMilli()
	for _, r := range batch {
		if _, err := stmt.ExecContext(ctx,
			r.ID, r.Queue, r.Name, r.State, r.Attempts, r.ExecutionDepth, r.TraceID,
			r.CreatedAtMs, r.FinishedAtMs, r.LastError, r.Data, r.Opts, now,
		); err != nil {
			_ = tx.Rollback()
			return err
		}
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	metrics.StoreWriteBatchDuration.Observe(time.Since(start).Seconds())
	metrics.StoreWritesTotal.Add(float64(len(batch)))
	return nil
}

// Search returns rows matching the given predicates, newest first.
func (s *Store) Search(ctx context.Context, p SearchParams) ([]SearchRow, error) {
	limit := p.Limit
	if limit <= 0 {
		limit = defaultLimit
	}
	if limit > maxLimit {
		limit = maxLimit
	}

	var (
		sb   strings.Builder
		args []any
	)
	sb.WriteString(`SELECT jh.id, jh.name, jh.state, jh.attempts, jh.execution_depth,
		jh.trace_id, jh.created_at, jh.finished_at, jh.last_error, jh.queue
		FROM job_history jh`)

	ftsQuery := buildFTSQuery(p.Query)
	if ftsQuery != "" {
		sb.WriteString(` JOIN job_fts f ON f.rowid = jh.rowid AND job_fts MATCH ?`)
		args = append(args, ftsQuery)
	}

	sb.WriteString(` WHERE 1=1`)
	if p.Name != "" {
		sb.WriteString(` AND jh.name = ?`)
		args = append(args, p.Name)
	}
	if p.State != "" {
		sb.WriteString(` AND jh.state = ?`)
		args = append(args, titleState(p.State))
	}
	if p.TraceID != "" {
		sb.WriteString(` AND jh.trace_id = ?`)
		args = append(args, p.TraceID)
	}
	if p.SinceMs > 0 {
		sb.WriteString(` AND jh.finished_at >= ?`)
		args = append(args, p.SinceMs)
	}
	sb.WriteString(` ORDER BY (CASE WHEN jh.finished_at > 0 THEN jh.finished_at ELSE jh.created_at END) DESC LIMIT ?`)
	args = append(args, limit)

	start := time.Now()
	rows, err := s.db.QueryContext(ctx, sb.String(), args...)
	metrics.StoreQueryDuration.WithLabelValues("search").Observe(time.Since(start).Seconds())
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]SearchRow, 0, limit)
	for rows.Next() {
		var r SearchRow
		if err := rows.Scan(&r.ID, &r.Name, &r.State, &r.Attempts, &r.ExecutionDepth,
			&r.TraceID, &r.CreatedAt, &r.FinishedAt, &r.LastError, &r.Queue); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// ErrNotFound is returned by Get when no job matches the id.
var ErrNotFound = errors.New("job not found")

// Get returns the full detail for a single job id. When multiple queues share
// an id, the most recently updated row wins.
func (s *Store) Get(ctx context.Context, id string) (*JobDetail, error) {
	const q = `SELECT jh.id, jh.name, jh.state, jh.attempts, jh.execution_depth,
		jh.trace_id, jh.created_at, jh.finished_at, jh.last_error, jh.queue, jh.data, jh.opts
		FROM job_history jh WHERE jh.id = ? ORDER BY jh.updated_at DESC LIMIT 1`

	start := time.Now()
	row := s.db.QueryRowContext(ctx, q, id)
	metrics.StoreQueryDuration.WithLabelValues("get").Observe(time.Since(start).Seconds())

	var d JobDetail
	err := row.Scan(&d.ID, &d.Name, &d.State, &d.Attempts, &d.ExecutionDepth,
		&d.TraceID, &d.CreatedAt, &d.FinishedAt, &d.LastError, &d.Queue, &d.Data, &d.Opts)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &d, nil
}

// Sweep enforces tiered retention: failed jobs are kept longer than
// completed/other jobs, and an optional hard row cap trims the oldest rows.
// It returns the total number of rows deleted.
func (s *Store) Sweep(ctx context.Context, r Retention) (int64, error) {
	now := time.Now()
	var total int64

	del := func(where string, args ...any) error {
		res, err := s.db.ExecContext(ctx, `DELETE FROM job_history WHERE `+where, args...)
		if err != nil {
			return err
		}
		n, _ := res.RowsAffected()
		total += n
		metrics.StoreSweepDeleted.Add(float64(n))
		return nil
	}

	if r.FailedTTL > 0 {
		cutoff := now.Add(-r.FailedTTL).UnixMilli()
		if err := del(
			`state = 'Failed' AND (CASE WHEN finished_at > 0 THEN finished_at ELSE created_at END) < ?`,
			cutoff,
		); err != nil {
			return total, err
		}
	}
	if r.CompletedTTL > 0 {
		cutoff := now.Add(-r.CompletedTTL).UnixMilli()
		if err := del(
			`state <> 'Failed' AND (CASE WHEN finished_at > 0 THEN finished_at ELSE created_at END) < ?`,
			cutoff,
		); err != nil {
			return total, err
		}
	}
	if r.MaxRows > 0 {
		if err := del(
			`rowid IN (
				SELECT rowid FROM job_history
				ORDER BY (CASE WHEN finished_at > 0 THEN finished_at ELSE created_at END) DESC
				LIMIT -1 OFFSET ?
			)`,
			r.MaxRows,
		); err != nil {
			return total, err
		}
	}
	return total, nil
}

// Close stops the writer (callers should cancel Run's context first) and
// closes the database handle.
func (s *Store) Close() error {
	select {
	case <-s.done:
	case <-time.After(10 * time.Second):
	}
	return s.db.Close()
}

// buildFTSQuery converts free user text into a safe FTS5 MATCH expression.
// Each whitespace token is quoted (so punctuation is literal) and prefix
// matched; tokens are ANDed. Returns "" when there is nothing to match.
func buildFTSQuery(q string) string {
	q = strings.TrimSpace(q)
	if q == "" {
		return ""
	}
	fields := strings.Fields(q)
	parts := make([]string, 0, len(fields))
	for _, f := range fields {
		f = strings.ReplaceAll(f, `"`, `""`)
		parts = append(parts, `"`+f+`"*`)
	}
	return strings.Join(parts, " ")
}
