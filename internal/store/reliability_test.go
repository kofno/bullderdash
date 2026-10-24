package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/kofno/bullderdash/internal/metrics"
	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
)

func metricValue(m prometheus.Metric) float64 {
	var value dto.Metric
	if err := m.Write(&value); err != nil {
		panic(err)
	}

	if value.Gauge != nil {
		return value.Gauge.GetValue()
	}
	return value.Counter.GetValue()
}

func histogramCount(m prometheus.Metric) uint64 {
	var value dto.Metric
	if err := m.Write(&value); err != nil {
		panic(err)
	}
	return value.Histogram.GetSampleCount()
}

func quietStore(t testing.TB, cfg Config) *Store {
	t.Helper()
	cfg.Path = filepath.Join(t.TempDir(), "history.db")
	s, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.db.Close() })
	return s
}

func seedRecords(t testing.TB, s *Store, n int) {
	t.Helper()
	now := time.Now().UnixMilli()
	payload := `{"device":"edge-router","action":"sync","details":"` + strings.Repeat("network telemetry device configuration ", 32) + `"}`
	for offset := 0; offset < n; offset += 512 {
		batch := make([]Record, 0, 512)
		for i := offset; i < min(offset+512, n); i++ {
			state := "Completed"
			if i%10 == 0 {
				state = "Failed"
			}
			batch = append(batch, Record{ID: fmt.Sprint(i), Queue: "q", Name: "sync",
				State: state, CreatedAtMs: now, FinishedAtMs: now, Data: payload})
		}
		if err := s.writeBatch(context.Background(), batch); err != nil {
			t.Fatal(err)
		}
	}
}

func plan(t testing.TB, s *Store, query string, args ...any) string {
	t.Helper()
	rows, err := s.db.Query("EXPLAIN QUERY PLAN "+query, args...)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var details []string
	for rows.Next() {
		var id, parent, unused int
		var detail string
		if err := rows.Scan(&id, &parent, &unused, &detail); err != nil {
			t.Fatal(err)
		}
		details = append(details, detail)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return strings.Join(details, "\n")
}

func TestRetentionPlansRealSchema(t *testing.T) {
	s := quietStore(t, Config{})
	seedRecords(t, s, 2000)
	// Include the obsolete competing index too: upgrading must not depend on
	// dropping it to make the new range indexes useful.
	if _, err := s.db.Exec(`CREATE INDEX idx_job_history_sweep ON job_history(state, finished_at, created_at)`); err != nil {
		t.Fatal(err)
	}
	for _, tier := range []struct{ where, index string }{
		{failedRetentionSQL, "idx_job_history_failed_end"},
		{completedRetentionSQL, "idx_job_history_completed_end"},
	} {
		detail := plan(t, s, sweepDeleteSQL(tier.index, tier.where), 1, 10)
		t.Log(detail)
		if !strings.Contains(detail, "SEARCH job_history USING INDEX "+tier.index+" (<expr><?)") {
			t.Fatalf("retention must range-seek its own tier, got %s", detail)
		}
	}
	detail := plan(t, s, `SELECT rowid FROM job_history ORDER BY `+effectiveEndSQL+` DESC LIMIT -1 OFFSET ?`, 100)
	if !strings.Contains(detail, "USING COVERING INDEX idx_job_history_end") &&
		!strings.Contains(detail, "USING INDEX idx_job_history_end") {
		t.Fatalf("hard cap must reuse the global end-time index: %s", detail)
	}
}

func TestMigrationPreservesHistoryAndFTS(t *testing.T) {
	s := quietStore(t, Config{})
	seedRecords(t, s, 20)
	if err := s.writeBatch(context.Background(), []Record{{ID: "fallback", Queue: "q", State: "Completed",
		CreatedAtMs: 1, Data: `{"message":"telemetry legacytoken"}`}}); err != nil {
		t.Fatal(err)
	}
	// Restore the pre-upgrade index layout around real rows and FTS triggers.
	for _, q := range []string{
		`DROP INDEX idx_job_history_end`,
		`DROP INDEX idx_job_history_failed_end`,
		`DROP INDEX idx_job_history_completed_end`,
		`CREATE INDEX idx_job_history_sweep ON job_history(state, finished_at, created_at)`,
	} {
		if _, err := s.db.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	var before int64
	if err := s.db.QueryRow(`SELECT total_changes()`).Scan(&before); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if err := migrate(s.db); err != nil {
			t.Fatal(err)
		}
	}
	var after int64
	if err := s.db.QueryRow(`SELECT total_changes()`).Scan(&after); err != nil {
		t.Fatal(err)
	}
	if after != before {
		t.Fatalf("migration rewrote rows/FTS: changes %d -> %d", before, after)
	}
	var obsolete int
	if err := s.db.QueryRow(`SELECT count(*) FROM sqlite_master WHERE name='idx_job_history_sweep'`).Scan(&obsolete); err != nil || obsolete != 0 {
		t.Fatalf("obsolete index: %d, %v", obsolete, err)
	}
	assertFTS(t, s)
	rows, err := s.Search(context.Background(), SearchParams{Query: "telemetry", Limit: 100})
	if err != nil || len(rows) != 21 {
		t.Fatalf("migrated search: %d rows, %v", len(rows), err)
	}
	if err := s.writeBatch(context.Background(), []Record{{ID: "fallback", Queue: "q", State: "Completed",
		Data: `{"message":"replacementtoken"}`}}); err != nil {
		t.Fatal(err)
	}
	rows, err = s.Search(context.Background(), SearchParams{Query: "legacytoken"})
	if err != nil || len(rows) != 0 {
		t.Fatalf("old FTS tokens survived upsert: %d rows, %v", len(rows), err)
	}
	if n, err := s.Sweep(context.Background(), Retention{CompletedTTL: time.Hour}); err != nil || n != 1 {
		t.Fatalf("migrated zero-finished fallback: deleted=%d err=%v", n, err)
	}
	assertFTS(t, s)
}

func assertFTS(t testing.TB, s *Store) {
	t.Helper()
	if _, err := s.db.Exec(`INSERT INTO job_fts(job_fts, rank) VALUES ('integrity-check', 1)`); err != nil {
		t.Fatalf("FTS integrity: %v", err)
	}
}

func TestRetentionFallbackAndDisabled(t *testing.T) {
	s := quietStore(t, Config{})
	now := time.Now().UnixMilli()
	old := now - int64((48 * time.Hour).Milliseconds())
	recs := []Record{
		{ID: "completed-fallback", State: "Completed", CreatedAtMs: old},
		{ID: "failed-fallback", State: "Failed", CreatedAtMs: old},
		{ID: "active-fallback", State: "Active", CreatedAtMs: old},
		{ID: "completed-finished", State: "Completed", CreatedAtMs: old, FinishedAtMs: now},
		{ID: "failed-finished", State: "Failed", CreatedAtMs: old, FinishedAtMs: now + 1},
	}
	if err := s.writeBatch(context.Background(), recs); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		r    Retention
		want int64
	}{
		{Retention{}, 0},
		{Retention{CompletedTTL: time.Hour, BatchSize: 1, Pause: -1}, 2},
		{Retention{FailedTTL: time.Hour, BatchSize: 1, Pause: -1}, 1},
		{Retention{MaxRows: 1, BatchSize: 1, Pause: -1}, 1},
	} {
		n, err := s.Sweep(context.Background(), tc.r)
		if err != nil || n != tc.want {
			t.Fatalf("Sweep %+v: deleted=%d want=%d err=%v", tc.r, n, tc.want, err)
		}
		assertFTS(t, s)
	}
	rows, err := s.Search(context.Background(), SearchParams{State: "Failed"})
	if err != nil || len(rows) != 1 || rows[0].ID != "failed-finished" {
		t.Fatalf("newest row not preserved: %+v %v", rows, err)
	}
}

func externalLock(t *testing.T, s *Store) (*sql.Conn, *sql.Tx) {
	t.Helper()
	conn, err := s.db.Conn(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	tx, err := conn.BeginTx(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(`UPDATE job_history SET updated_at=updated_at`); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = tx.Rollback(); _ = conn.Close() })
	return conn, tx
}

func TestContentionRetriesExactBatchAndAllowsReaders(t *testing.T) {
	s := quietStore(t, Config{})
	seedRecords(t, s, 10)
	_, tx := externalLock(t, s)
	// Pin the other pool connection and shorten only its busy timeout so this
	// test exercises retries quickly; production keeps its existing 5000ms.
	conn, err := s.db.Conn(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := conn.ExecContext(context.Background(), `PRAGMA busy_timeout=1`); err != nil {
		t.Fatal(err)
	}
	_ = conn.Close()
	before := metricValue(metrics.StoreWriteRetries.WithLabelValues("ingest"))
	batch := []Record{
		{ID: "retry-a", Queue: "q", State: "Completed", Data: `{"message":"retrytoken"}`},
		{ID: "retry-b", Queue: "q", State: "Failed", Data: `{"message":"retrytoken"}`},
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	result := make(chan error, 1)
	go func() { result <- s.writeBatch(ctx, batch) }()
	deadline := time.Now().Add(time.Second)
	for metricValue(metrics.StoreWriteRetries.WithLabelValues("ingest")) == before {
		if time.Now().After(deadline) {
			t.Fatal("no retry observed")
		}
		time.Sleep(time.Millisecond)
	}
	if rows, err := s.Search(ctx, SearchParams{State: "Completed"}); err != nil || len(rows) != 9 {
		t.Fatalf("WAL reader blocked by writer: %d %v", len(rows), err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	if err := <-result; err != nil {
		t.Fatal(err)
	}
	rows, err := s.Search(ctx, SearchParams{Query: "retrytoken"})
	if err != nil || len(rows) != 2 {
		t.Fatalf("retried batch lost: %+v %v", rows, err)
	}
	assertFTS(t, s)
}

func TestWriteAdmissionAndSweepPacingCancellation(t *testing.T) {
	s := quietStore(t, Config{})
	s.writeGate <- struct{}{}
	for _, operation := range []func(context.Context) error{
		func(ctx context.Context) error { return s.writeBatch(ctx, []Record{{ID: "blocked"}}) },
		func(ctx context.Context) error {
			_, err := s.Sweep(ctx, Retention{CompletedTTL: time.Hour})
			return err
		},
	} {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
		start := time.Now()
		err := operation(ctx)
		cancel()
		if !errors.Is(err, context.DeadlineExceeded) || time.Since(start) > time.Second {
			t.Fatalf("admission cancellation: %v after %s", err, time.Since(start))
		}
	}
	<-s.writeGate
	if err := s.writeBatch(context.Background(), []Record{{ID: "old", State: "Completed", CreatedAtMs: 1}}); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	n, err := s.Sweep(ctx, Retention{CompletedTTL: time.Hour, BatchSize: 1, Pause: time.Hour})
	if n != 1 || !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("pacing cancellation: deleted=%d err=%v", n, err)
	}
}

func TestQueueMetricsAndPermanentLoss(t *testing.T) {
	s := quietStore(t, Config{WriteBuffer: 2, BatchSize: 1})
	beforeDrops := metricValue(metrics.StoreWritesDropped)
	beforeLost := metricValue(metrics.StoreWriteLost)
	beforeErrors := metricValue(metrics.StoreWriteErrors)
	for i := 0; i < 3; i++ {
		s.Enqueue(Record{ID: fmt.Sprint(i)})
	}
	if metricValue(metrics.StoreQueueDepth) != 2 ||
		metricValue(metrics.StoreQueueCapacity) != 2 ||
		metricValue(metrics.StoreWritesDropped)-beforeDrops != 1 {
		t.Fatal("incorrect full queue metrics")
	}
	if _, err := s.db.Exec(`DROP TABLE job_history`); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	go s.Run(ctx)
	select {
	case <-s.done:
	case <-time.After(time.Second):
		t.Fatal("permanent error hung shutdown")
	}
	if metricValue(metrics.StoreWriteLost)-beforeLost != 2 ||
		metricValue(metrics.StoreWriteErrors)-beforeErrors != 2 ||
		metricValue(metrics.StoreQueueDepth) != 0 {
		t.Fatal("incorrect permanent loss / drain metrics")
	}
}

func TestSweepYieldsToIngestion(t *testing.T) {
	s := quietStore(t, Config{})
	batch := make([]Record, 100)
	for i := range batch {
		batch[i] = Record{ID: fmt.Sprint(i), State: "Completed", CreatedAtMs: 1, Data: `{"value":"expiredtoken"}`}
	}
	if err := s.writeBatch(context.Background(), batch); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	result := make(chan error, 1)
	go func() {
		_, err := s.Sweep(ctx, Retention{CompletedTTL: time.Hour, BatchSize: 1, Pause: 50 * time.Millisecond})
		result <- err
	}()
	deadline := time.Now().Add(time.Second)
	for {
		var remaining int
		if err := s.db.QueryRow(`SELECT count(*) FROM job_history`).Scan(&remaining); err != nil {
			t.Fatal(err)
		}
		if remaining < 100 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("sweep did not start")
		}
		time.Sleep(time.Millisecond)
	}
	writeCtx, stop := context.WithTimeout(context.Background(), time.Second)
	defer stop()
	if err := s.writeBatch(writeCtx, []Record{{ID: "fresh", State: "Completed", FinishedAtMs: time.Now().UnixMilli()}}); err != nil {
		t.Fatalf("sweep starved ingestion: %v", err)
	}
	select {
	case err := <-result:
		t.Fatalf("sweep ended before ingestion progressed: %v", err)
	default:
	}
	cancel()
	if err := <-result; !errors.Is(err, context.Canceled) {
		t.Fatalf("sweep cancellation: %v", err)
	}
	assertFTS(t, s)
}

func TestShutdownRetriesPendingBatch(t *testing.T) {
	s := quietStore(t, Config{WriteBuffer: 16, BatchSize: 2, FlushInterval: time.Millisecond})
	seedRecords(t, s, 1)
	_, tx := externalLock(t, s)
	ctx, cancel := context.WithCancel(context.Background())
	for i := 0; i < 8; i++ {
		s.Enqueue(Record{ID: fmt.Sprintf("pending-%d", i), State: "Failed"})
	}
	go s.Run(ctx)
	// Cancellation interrupts the original attempt, then shutdown retries the
	// retained batch using one fresh drain context.
	time.Sleep(20 * time.Millisecond)
	cancel()
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	select {
	case <-s.done:
	case <-time.After(7 * time.Second):
		t.Fatal("shutdown did not finish")
	}
	var n int
	if err := s.db.QueryRow(`SELECT count(*) FROM job_history WHERE id LIKE 'pending-%'`).Scan(&n); err != nil || n != 8 {
		t.Fatalf("interrupted batch or queued records lost: n=%d err=%v", n, err)
	}
}

func TestShutdownDeadlineSharedAcrossBatches(t *testing.T) {
	s := quietStore(t, Config{WriteBuffer: 8, BatchSize: 1})
	s.writeGate <- struct{}{}
	before := metricValue(metrics.StoreWriteLost)
	for i := 0; i < 8; i++ {
		s.Enqueue(Record{ID: fmt.Sprint(i)})
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	start := time.Now()
	go s.Run(ctx)
	select {
	case <-s.done:
	case <-time.After(7 * time.Second):
		t.Fatal("shutdown applied a separate timeout to each batch")
	}
	<-s.writeGate
	if elapsed := time.Since(start); elapsed < 4*time.Second || elapsed > 7*time.Second {
		t.Fatalf("unexpected drain bound: %s", elapsed)
	}
	if lost := metricValue(metrics.StoreWriteLost) - before; lost != 8 {
		t.Fatalf("shutdown loss count=%v want 8", lost)
	}
}

func TestBusyCancellation(t *testing.T) {
	s := quietStore(t, Config{})
	seedRecords(t, s, 1)
	_, _ = externalLock(t, s)
	conn, err := s.db.Conn(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := conn.ExecContext(context.Background(), `PRAGMA busy_timeout=1`); err != nil {
		t.Fatal(err)
	}
	_ = conn.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Millisecond)
	defer cancel()
	start := time.Now()
	err = s.writeBatch(ctx, []Record{{ID: "cancelled"}})
	if !errors.Is(err, context.DeadlineExceeded) || time.Since(start) > time.Second {
		t.Fatalf("retry cancellation: %v after %s", err, time.Since(start))
	}
}

func TestImmediateTransactionReaderBehavior(t *testing.T) {
	s := quietStore(t, Config{})
	seedRecords(t, s, 1)
	_, _ = externalLock(t, s)
	db, err := sql.Open("sqlite", s.cfg.Path+"?_pragma=busy_timeout(1)&_txlock=immediate")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	tx, err := db.BeginTx(context.Background(), &sql.TxOptions{ReadOnly: true})
	if err != nil {
		t.Fatalf("explicit read-only transaction must not acquire a writer lock: %v", err)
	}
	_ = tx.Rollback()
	tx, err = db.BeginTx(context.Background(), nil)
	if err == nil {
		_ = tx.Rollback()
		t.Fatal("default immediate transaction should require a writer lock")
	}
	if !retryable(err) {
		t.Fatalf("expected a typed SQLite contention error, got %v", err)
	}
	// Plain SELECTs also read WAL. We keep deferred transactions: our mutations
	// write first, with no read-snapshot upgrade that would need immediate mode.
	var n int
	if err := db.QueryRow(`SELECT count(*) FROM job_history`).Scan(&n); err != nil || n != 1 {
		t.Fatalf("WAL SELECT: %d %v", n, err)
	}
}

func TestConcurrentQueueMetrics(t *testing.T) {
	s := quietStore(t, Config{WriteBuffer: 512, BatchSize: 16, FlushInterval: time.Millisecond})
	ctx, cancel := context.WithCancel(context.Background())
	go s.Run(ctx)
	var producers sync.WaitGroup
	for p := 0; p < 4; p++ {
		producers.Add(1)
		go func(p int) {
			defer producers.Done()
			for i := 0; i < 100; i++ {
				s.Enqueue(Record{ID: fmt.Sprintf("%d-%d", p, i), State: "Completed"})
			}
		}(p)
	}
	producers.Wait()
	cancel()
	<-s.done
	if metricValue(metrics.StoreQueueDepth) != 0 || len(s.recs) != 0 {
		t.Fatal("queue gauge stale after concurrent producers and drain")
	}
}

func TestBackloggedQueueMetrics(t *testing.T) {
	s := quietStore(t, Config{WriteBuffer: 8, BatchSize: 1})
	s.writeGate <- struct{}{}
	ctx, cancel := context.WithCancel(context.Background())
	go s.Run(ctx)
	s.Enqueue(Record{ID: "inflight"})
	deadline := time.Now().Add(time.Second)
	for len(s.recs) != 0 {
		if time.Now().After(deadline) {
			t.Fatal("writer did not receive first record")
		}
		time.Sleep(time.Millisecond)
	}
	for i := 0; i < 8; i++ {
		s.Enqueue(Record{ID: fmt.Sprint(i)})
	}
	if metricValue(metrics.StoreQueueDepth) != 8 {
		t.Fatal("queue gauge stale while writer blocked")
	}
	<-s.writeGate
	cancel()
	<-s.done
}

func TestTimingIncludesFailuresAndSeparatesAdmission(t *testing.T) {
	s := quietStore(t, Config{})
	ingestBefore := histogramCount(metrics.StoreWriteBatchDuration)
	sweepBefore := histogramCount(metrics.StoreSweepDuration)
	deleteBefore := histogramCount(metrics.StoreSweepBatchDuration)
	wait := metrics.StoreWriteLockWait.WithLabelValues("sweep").(prometheus.Histogram)
	waitBefore := histogramCount(wait)
	s.writeGate <- struct{}{}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	_, err := s.Sweep(ctx, Retention{CompletedTTL: time.Hour})
	<-s.writeGate
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal(err)
	}
	if histogramCount(metrics.StoreSweepDuration) != sweepBefore+1 ||
		histogramCount(wait) != waitBefore+1 ||
		histogramCount(metrics.StoreSweepBatchDuration) != deleteBefore {
		t.Fatal("admission-only failure counted as SQLite DELETE time")
	}
	if _, err := s.db.Exec(`DROP TABLE job_history`); err != nil {
		t.Fatal(err)
	}
	if err := s.writeBatch(context.Background(), []Record{{ID: "broken"}}); err == nil {
		t.Fatal("expected permanent ingest error")
	}
	if _, err := s.Sweep(context.Background(), Retention{CompletedTTL: time.Hour}); err == nil {
		t.Fatal("expected permanent sweep error")
	}
	if histogramCount(metrics.StoreWriteBatchDuration) != ingestBefore+1 ||
		histogramCount(metrics.StoreSweepDuration) != sweepBefore+2 ||
		histogramCount(metrics.StoreSweepBatchDuration) != deleteBefore+1 {
		t.Fatal("failed attempts missing from timing metrics")
	}
}

func TestExtendedBusySnapshotIsRetryable(t *testing.T) {
	s := quietStore(t, Config{})
	seedRecords(t, s, 1)
	tx, err := s.db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	var n int
	if err := tx.QueryRow(`SELECT count(*) FROM job_history`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if err := s.writeBatch(context.Background(), []Record{{ID: "new", State: "Completed"}}); err != nil {
		t.Fatal(err)
	}
	_, err = tx.Exec(`UPDATE job_history SET updated_at=1`)
	var sqliteErr interface{ Code() int }
	if !errors.As(err, &sqliteErr) || sqliteErr.Code() != 517 || !retryable(fmt.Errorf("wrapped: %w", err)) {
		t.Fatalf("expected retryable SQLITE_BUSY_SNAPSHOT (517), got %v", err)
	}
	if retryable(errors.New("permanent failure")) {
		t.Fatal("untyped error must not be retried")
	}
}

func TestSweepRetriesExactDeleteBatch(t *testing.T) {
	s := quietStore(t, Config{})
	if err := s.writeBatch(context.Background(), []Record{
		{ID: "old-a", State: "Failed", CreatedAtMs: 1, Data: "expiredtoken"},
		{ID: "old-b", State: "Completed", CreatedAtMs: 1, Data: "expiredtoken"},
	}); err != nil {
		t.Fatal(err)
	}
	_, tx := externalLock(t, s)
	conn, err := s.db.Conn(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := conn.ExecContext(context.Background(), `PRAGMA busy_timeout=1`); err != nil {
		t.Fatal(err)
	}
	_ = conn.Close()
	before := metricValue(metrics.StoreWriteRetries.WithLabelValues("sweep"))
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	result := make(chan error, 1)
	go func() {
		n, err := s.Sweep(ctx, Retention{CompletedTTL: time.Hour, FailedTTL: time.Hour, BatchSize: 1})
		if err == nil && n != 2 {
			err = fmt.Errorf("deleted=%d want 2", n)
		}
		result <- err
	}()
	deadline := time.Now().Add(time.Second)
	for metricValue(metrics.StoreWriteRetries.WithLabelValues("sweep")) == before {
		if time.Now().After(deadline) {
			t.Fatal("no sweep retry observed")
		}
		time.Sleep(time.Millisecond)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	if err := <-result; err != nil {
		t.Fatal(err)
	}
	assertFTS(t, s)
}

func TestDriverBusyTimeoutCancellationBound(t *testing.T) {
	s := quietStore(t, Config{})
	seedRecords(t, s, 1)
	_, _ = externalLock(t, s)
	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Millisecond)
	defer cancel()
	start := time.Now()
	err := s.writeBatch(ctx, []Record{{ID: "busy"}})
	elapsed := time.Since(start)
	t.Logf("default 5000ms busy timeout, 40ms context: cancellation returned in %s", elapsed)
	if err == nil || elapsed > 6*time.Second {
		t.Fatalf("busy cancellation unbounded: %v after %s", err, elapsed)
	}
}

func TestEnqueueAfterShutdownIsCountedNotAccepted(t *testing.T) {
	s := quietStore(t, Config{})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	go s.Run(ctx)
	<-s.done
	before := metricValue(metrics.StoreWriteLost)
	s.Enqueue(Record{ID: "late"})
	if len(s.recs) != 0 || metricValue(metrics.StoreQueueDepth) != 0 ||
		metricValue(metrics.StoreWriteLost)-before != 1 {
		t.Fatal("post-shutdown record accepted or silently lost")
	}
}

// Opt-in bounded local workload; never uses a production DB or Redis.
func TestHistoryReliabilitySoak(t *testing.T) {
	if os.Getenv("STORE_RELIABILITY_SOAK") != "1" {
		t.Skip("set STORE_RELIABILITY_SOAK=1 for retained-row contention workload")
	}
	for _, retained := range []int{10_000, 100_000} {
		t.Run(fmt.Sprint(retained), func(t *testing.T) {
			s := quietStore(t, Config{WriteBuffer: 16384, BatchSize: 512, FlushInterval: 500 * time.Millisecond})
			seedStart := time.Now()
			seedRecords(t, s, retained)
			t.Logf("seeded %d mixed retained rows with ~1.2KiB indexed JSON in %s", retained, time.Since(seedStart))
			expired := make([]Record, 4000)
			for i := range expired {
				expired[i] = Record{ID: fmt.Sprintf("old-%d", i), Queue: "q", State: "Completed",
					CreatedAtMs: 1, Data: `{"message":"` + strings.Repeat("expired telemetry payload ", 48) + `"}`}
			}
			for offset := 0; offset < len(expired); offset += 512 {
				if err := s.writeBatch(context.Background(), expired[offset:min(offset+512, len(expired))]); err != nil {
					t.Fatal(err)
				}
			}
			beforeDrops := metricValue(metrics.StoreWritesDropped)
			beforeLost := metricValue(metrics.StoreWriteLost)
			ctx, cancel := context.WithCancel(context.Background())
			go s.Run(ctx)
			defer func() { cancel(); <-s.done }()
			sweepCtx, stopSweep := context.WithTimeout(context.Background(), 15*time.Second)
			defer stopSweep()
			sweepResult := make(chan error, 1)
			go func() {
				n, err := s.Sweep(sweepCtx, Retention{CompletedTTL: time.Hour, FailedTTL: 14 * 24 * time.Hour,
					BatchSize: 100, Pause: 25 * time.Millisecond})
				if err == nil && n != 4000 {
					err = fmt.Errorf("deleted=%d want 4000", n)
				}
				sweepResult <- err
			}()
			// External write lock overlaps ingestion and sweep, using the real
			// production 5s busy timeout, not the shortened unit-test timeout.
			conn, tx := externalLock(t, s)
			unlocked := make(chan error, 1)
			go func() {
				time.Sleep(200 * time.Millisecond)
				err := tx.Commit()
				_ = conn.Close()
				unlocked <- err
			}()
			payload := `{"traceId":"soaktrace","deviceId":"edge-42","readings":"` + strings.Repeat("voltage temperature status ", 48) + `"}`
			enqueued := 0
			maxDepth := 0
			send := func() {
				s.Enqueue(Record{ID: fmt.Sprintf("ingest-%d", enqueued), Name: "soak-ingest", Queue: "q",
					State: "Completed", FinishedAtMs: time.Now().UnixMilli(), Data: payload})
				enqueued++
				maxDepth = max(maxDepth, len(s.recs))
			}
			start := time.Now()
			for i := 0; i < 2048; i++ {
				send()
			}
			ticker := time.NewTicker(time.Second / 74)
			end := time.NewTimer(5 * time.Second)
		ingest:
			for {
				select {
				case <-ticker.C:
					send()
				case <-end.C:
					break ingest
				}
			}
			ticker.Stop()
			if err := <-unlocked; err != nil {
				t.Fatal(err)
			}
			cancel()
			<-s.done
			if err := <-sweepResult; err != nil {
				t.Fatal(err)
			}
			var persisted int
			if err := s.db.QueryRow(`SELECT count(*) FROM job_history WHERE name='soak-ingest'`).Scan(&persisted); err != nil {
				t.Fatal(err)
			}
			drops := metricValue(metrics.StoreWritesDropped) - beforeDrops
			lost := metricValue(metrics.StoreWriteLost) - beforeLost
			t.Logf("retained=%d duration=%s offered=%d persisted=%d max_sampled_channel_depth=%d drops=%g lost=%g",
				retained, time.Since(start), enqueued, persisted, maxDepth, drops, lost)
			if persisted != enqueued || drops != 0 || lost != 0 {
				t.Fatal("soak lost records")
			}
			assertFTS(t, s)
		})
	}
}

func BenchmarkRetentionRangeSeek(b *testing.B) {
	for _, retained := range []int{10_000, 100_000} {
		b.Run(fmt.Sprint(retained), func(b *testing.B) {
			s := quietStore(b, Config{})
			seedRecords(b, s, retained)
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				n, err := s.Sweep(context.Background(), Retention{CompletedTTL: time.Hour, FailedTTL: time.Hour})
				if err != nil || n != 0 {
					b.Fatalf("empty sweep: n=%d err=%v", n, err)
				}
			}
		})
	}
}
