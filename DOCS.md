# Bull-der-dash User Guide

## Quick Links
- `QUICKSTART.md` for a fast setup
- `README.md` for project overview
- `scripts/sim/README.md` for simulator details
- `ARCHITECTURE.md` for system design

## Using the UI

### Dashboard
- URL: `http://localhost:8080`
- Auto-refreshes every 5s
- Click any count to open a state-specific job list

### Queue Details
- URL: `http://localhost:8080/queue/<name>`
- Shows summary counts and job lists per state

### Job Detail
- URL: `http://localhost:8080/job/detail?queue=<name>&id=<id>`
- JSON view of full job data

### Search Console
- URL: `http://localhost:8080/console`
- Available when `STORE_ENABLED=true` (populated only when `WORKLOAD_METRICS_ENABLED=true`)
- Full-text search over persisted job history (name, trace id, last error, payloads)
- Filter by queue, job name, state, trace id, and time window
- Drill down into a trace's lineage and open full single-job detail
- Dependency-free page served from the embedded SQLite FTS5 store — it never scans live Redis

## Endpoints

- `GET /` - Dashboard
- `GET /assets/` - Embedded shared CSS/JS (Anvil design system)
- `GET /queues` - Polled HTML fragment: queue list (served from the background snapshot)
- `GET /queue/<name>` - Queue detail view
- `GET /queue/summary?queue=<name>` - Polled HTML fragment: single-queue summary
- `GET /queue/jobs?queue=<name>&state=<state>&page=<n>` - Paged state job list (`state=all` for every state)
- `GET /job/detail?queue=<name>&id=<id>` - Job detail (JSON)
- `GET /search` - Deprecated; redirects to `/console` (or `/` when the store is disabled)
- `GET /console` - Full-text search console (when `STORE_ENABLED=true`)
- `GET /v1/search?q=&name=&state=&trace_id=&errored=&since_ms=&limit=` - Search persisted history (JSON; requires at least one of `q`/`name`/`state`/`trace_id`/`errored`/`since_ms`, else `400`; `503` when readers are saturated)
- `GET /v1/jobs/{id}` - Persisted detail for one job (JSON; `404` once it has aged out of retention)
- `GET /v1/stats/job-names?queue=&state=&since_ms=&limit=` - Exact top job names from history (JSON)
- `GET /metrics` - Prometheus metrics
- `GET /health`, `/healthz`, `/ready`, `/readyz` - Health checks

## Metrics

Queue depth metrics:
- `bullmq_queue_waiting{queue="..."}`
- `bullmq_queue_active{queue="..."}`
- `bullmq_queue_paused{queue="..."}`
- `bullmq_queue_prioritized{queue="..."}`
- `bullmq_queue_waiting_children{queue="..."}`
- `bullmq_queue_completed{queue="..."}`
- `bullmq_queue_failed{queue="..."}`
- `bullmq_queue_delayed{queue="..."}`
- `bullmq_queue_stalled{queue="..."}`
- `bullmq_queue_orphaned{queue="..."}`

Service metrics:
- `http_request_duration_seconds{method,path,status}` (path is normalized to stable routes)
- `redis_operation_duration_seconds{operation}`
- `redis_operation_errors_total{operation}`

Workload metrics, when `WORKLOAD_METRICS_ENABLED=true`:
- `bullmq_jobs_finished_total{queue,name,result}` - Observed completed/failed jobs by queue, job name, and result
- `bullmq_job_completion_duration_seconds{queue,name,result}` - Histogram of `finishedOn - processedOn`
- `bullmq_workload_event_lag_seconds{queue}` - Approximate age of the latest observed BullMQ event stream entry
- `bullmq_workload_events_read_total{queue,event}` - BullMQ event stream entries read by the collector
- `bullmq_workload_events_dropped_total{queue,reason}` - Terminal events skipped because the event itself was missing required fields
- `bullmq_workload_job_lookup_errors_total{queue,reason}` - Job hash lookup or parsing failures

History store metrics, when `STORE_ENABLED=true`:
- `bullderdash_store_writes_total` - Records persisted to SQLite
- `bullderdash_store_writes_dropped_total` - Records dropped because the `STORE_WRITE_BUFFER` channel was full (history gaps)
- `bullderdash_store_write_errors_total` - Failed write batches (every record in the batch is lost, e.g. when a batch times out waiting for the write lock)
- `bullderdash_store_write_batch_duration_seconds` - Write-batch latency
- `bullderdash_store_query_duration_seconds{operation}` - Initial query-call latency (`search`, `get`, `stats_job_names`); does not include reading all results
- `bullderdash_store_sweep_deleted_total` - Rows removed by the retention sweeper

## Configuration

Environment variables:
- `REDIS_ADDR` (default `127.0.0.1:6379`)
- `REDIS_USERNAME` (default empty)
- `REDIS_PASSWORD` (default empty)
- `REDIS_DB` (default `0`)
- `REDIS_SENTINEL_MASTER` (default empty)
- `REDIS_SENTINEL_ADDRS` (default empty, comma-separated)
- `REDIS_SENTINEL_USERNAME` (default empty)
- `REDIS_SENTINEL_PASSWORD` (default empty)
- `SERVER_PORT` (default `8080`)
- `QUEUE_PREFIX` (default `bull`)
- `METRICS_POLL_SECONDS` (default `10`)
- `DASHBOARD_REFRESH_TIMEOUT_SECONDS` (default `30`)
- `WORKLOAD_METRICS_ENABLED` (default `false`)
- `WORKLOAD_METRICS_POLL_SECONDS` (default `10`)
- `WORKLOAD_METRICS_BLOCK_SECONDS` (default `1`)
- `WORKLOAD_METRICS_BATCH_SIZE` (default `100`)
- `WORKLOAD_METRICS_MAX_JOB_NAMES_PER_QUEUE` (default `100`)
- `WORKLOAD_METRICS_START_ID` (default `$`)
- `STORE_ENABLED` (default `false`) - persist completed/failed jobs to SQLite and enable `/console`; requires `WORKLOAD_METRICS_ENABLED=true`, since the workload collector is what feeds the store
- `STORE_DB_PATH` (default `/data/history.db`) - SQLite file path; its directory must be writable
- `STORE_WRITE_BUFFER` (default `4096`) - in-memory write channel size; records drop when full
- `STORE_BATCH_SIZE` (default `256`) - max records per write transaction
- `STORE_FLUSH_MILLIS` (default `500`) - max time a batch waits before flushing
- `STORE_TRACE_KEYS` (default empty) - comma-separated payload keys used to derive a trace id
- `STORE_COMPLETED_TTL_HOURS` (default `24`) - retention for non-failed history; `0` disables
- `STORE_FAILED_TTL_HOURS` (default `336`) - retention for failed history (14 days); `0` disables
- `STORE_SWEEP_SECONDS` (default `300`) - interval between retention sweeps
- `STORE_SWEEP_BATCH` (default `2000`) - rows deleted per sweep statement; small batches keep each DELETE (and its per-row FTS5 trigger work) fast so a large backlog drains across many small transactions instead of one oversized DELETE
- `STORE_SWEEP_PAUSE_MILLIS` (default `25`) - cancellation-aware pause between full DELETE batches; `0` uses the default, negative disables pacing
- `STORE_SWEEP_TIMEOUT_SECONDS` (default `0`) - optional per-sweep deadline; `0` means no artificial deadline (the sweep drains the eligible backlog in batches, cancelling only on shutdown)
- `STORE_MAX_ROWS` (default `0`) - optional hard cap on total history rows; `0` disables
- `STORE_READ_CONCURRENCY` (default `16`) - max concurrent search/detail reads; excess gets `503`
- `LOG_LEVEL` (default `info`)

Workload metrics are collected from BullMQ event streams in a background
goroutine. `/metrics` only exports in-memory Prometheus data; it does not scan
retained jobs or issue Redis commands during a scrape.

When `STORE_ENABLED=true`, terminal (completed/failed) jobs observed by the
workload collector are written to the embedded SQLite history store by a
background writer, and a sweeper ages them out per the `STORE_*_TTL_HOURS`
settings (failures are kept longer than successes). Search and job-detail reads
are served from SQLite, never from live Redis.

#### History write reliability and diagnosis

The background writer and retention DELETEs share cancellation-aware, in-process
write admission. Admission is released after every DELETE batch, including a
failed attempt; the sweeper pauses between full batches to let ingestion progress.
WAL readers still use separate connections. SQLite `BUSY`/`LOCKED` errors
(including extended codes) retry the **same** transaction batch with exponential
backoff from 10ms to a 250ms cap, until success or cancellation. Other errors are
not retried: the lost record count is logged and counted. Shutdown retries an
interrupted batch and drains the channel snapshot using one shared five-second
deadline, not five seconds per batch. A SQLite busy wait or in-flight statement
can delay cancellation; the existing driver busy timeout remains five seconds.
New enqueues after the shutdown snapshot are rejected, logged and counted as
lost, rather than accepted into a channel that no longer has a writer.
This is best-effort async persistence, not a durable queue: full-channel drops,
permanent errors, exhausted shutdown drains, and process crashes can lose history.
Collector cursors are still in-memory and default to `$` after restart.

On an existing database, startup builds expression indexes over the effective
end time (`finished_at` when positive, otherwise `created_at`): one global index
and separate failed/non-failed partial indexes. The one-time build reads retained
rows, requires disk space, and can delay startup; allow for this during rollout
without assuming a fixed duration. No record backfill or FTS rebuild is performed.
The obsolete `(state, finished_at, created_at)` sweep index is then removed; the
other search indexes remain. TTL DELETEs explicitly select their tier's range
index so competing state indexes cannot turn them back into retained-row scans.
Search ordering, zero-finished fallback, disabled TTL tiers and the optional
newest-first hard cap are unchanged; arbitrary search filters may still sort.

The **Collector & Service Internals** Grafana dashboard includes history panels:

| Signal | Interpretation / action |
|--------|-------------------------|
| `bullderdash_store_queue_depth` / `bullderdash_store_queue_capacity` | Channel backlog, excluding the in-flight batch; updated on enqueue/dequeue even while writes are blocked. Sustained growth means persistence is not keeping up. A larger buffer absorbs bursts, not sustained overload. |
| `bullderdash_store_write_lock_wait_seconds` (`operation`: `ingest` or `sweep`) | In-process admission wait per attempt, including cancelled waits; not SQLite execution time. High ingest wait with long DELETEs suggests trying a smaller sweep batch or more pacing. |
| `bullderdash_store_sweep_batch_duration_seconds` | Each SQLite DELETE attempt, including pool/SQLite busy wait and FTS trigger/commit work, on success **or error**. Excludes in-process admission and pacing. High values with low admission wait point to pool/SQLite/storage/FTS cost. |
| `bullderdash_store_sweep_duration_seconds` | Whole sweep on success or error, including admission, retries and pauses. Compare deleted-row rate with ingest, not just sweep wall time. |
| `bullderdash_store_write_retries_total` (`operation`: `ingest` or `sweep`) | Typed SQLite contention retries. Persistent growth can indicate another writer/maintenance process; distinguish this from internal admission wait. |
| `bullderdash_store_write_lost_records_total` | Records discarded on write error, exhausted shutdown drain or enqueue after shutdown. Check the accompanying `lost_records` error logs; separate from full-channel `bullderdash_store_writes_dropped_total`. |
| `bullderdash_store_write_batch_duration_seconds` | Ingest batch duration from write admission through the final attempt, including retries and failures but excluding channel/batch-formation wait (previous versions observed only successful batches). `write_errors_total` counts finally discarded batches, not retry attempts. |

The operation labels have only `ingest` and `sweep` values. No job IDs, queues or
payloads are added to these labels or error logs. These signals identify where
time is spent; a full-channel-drop alert alone does **not** identify the sweeper
as the cause.

Local regression workload (temporary databases only):
`go test ./internal/store -run TestHistoryReliabilitySoak -count=1 -v` with
`STORE_RELIABILITY_SOAK=1` compares 10k/100k mixed retained rows, ~1.2KiB indexed
payloads, ~74 records/s plus a 2048-record burst, a concurrent expiry backlog,
and an external writer lock. `go test ./internal/store -run '^$' -bench
BenchmarkRetentionRangeSeek -benchtime=1s` compares no-expiry sweep cost at
those retained sizes. These are bounded local checks, not production storage
benchmarks or a guarantee at multi-million-row retention.

An example local run (Windows/amd64, Ryzen 7 PRO 7840U, synthetic common-token
payloads, warm local storage) measured the following. The soak used a 16,384-record
channel, 512-record write batches, 500ms flush, **100-row sweep batches** with
25ms pacing, a 4,000-row expired backlog, and a 200ms external writer transaction.
The default sweep batch remains 2,000; smaller test batches deliberately exercise
interleaving.

| Retained rows (90% completed / 10% failed) | Offered / persisted in ~5s (74/s + burst) | Drops / error losses | Sampled peak channel depth | No-expiry sweep benchmark |
|-----------------------------------------|------------------------------------------|----------------------|----------------------------|---------------------------|
| 10,000 | 2,418 / 2,418 | 0 / 0 | 1,572 | 105.6us/op |
| 100,000 | 2,418 / 2,418 | 0 / 0 | 1,595 | 114.8us/op |

Both TTL plans range-seek their own partial expression index with all legacy
competing indexes present. FTS integrity checks passed after migration, upsert
and deletion. This does not measure managed-csi latency, cold-cache behavior,
production payload entropy, million-row startup index builds or long-running
WAL/checkpoint pressure; none of those should be inferred from these numbers.

#### Storage sizing and the FTS index

The history store is backed by an SQLite FTS5 full-text index (`job_fts`) over
the job `id`, `name`, `trace_id`, `last_error`, and the full `data` payload.
This has two consequences worth planning for:

- **The database is larger than the raw job data.** FTS5 keeps an inverted
  index of every indexed term, so indexed text — especially large JSON payloads
  stored in `data` and stacktraces in `last_error` — is effectively stored
  twice (once in the base row, once in the index). Expect the FTS index to be a
  substantial fraction of total file size, often roughly as large as the base
  table for payload-heavy workloads.

- **The file size plateaus at a steady state, it does not grow without bound.**
  Retained rows are bounded by `STORE_COMPLETED_TTL_HOURS` and
  `STORE_FAILED_TTL_HOURS`, so the resident row count stops growing once the
  oldest tier reaches its TTL (24h for completed by default). From then on the
  sweeper deletes about as fast as new jobs arrive. SQLite returns freed pages
  to an internal free-list and **reuses** them for new inserts rather than
  returning them to the OS, so the file holds near its high-water mark instead
  of shrinking. A high-throughput queue therefore reaches a stable size roughly
  equal to `completed_rate × completed_TTL` (plus the much smaller failed tier
  and the FTS index) within one `completed_TTL` window — the initial fast growth
  is this one-time ramp, not runaway growth.

- **The plateau assumes the sweeper keeps pace with ingest.** Each sweep deletes
  in bounded batches (`STORE_SWEEP_BATCH`, default 2000 rows per statement) and
  loops until the eligible backlog is drained, so one sweep can clear far more
  than a single batch. If sustained throughput outruns the sweeper (resident
  rows and file size keep climbing past one `completed_TTL`), use the history
  metrics above to distinguish infrequent sweeps, write admission pressure and
  slow SQLite work. Shorten `STORE_SWEEP_SECONDS` for infrequent sweeps; tune
  batch size and pacing together rather than blindly raising the batch size
  (which can starve ingestion). Watch `bullderdash_store_sweep_deleted_total`:
  it should keep climbing in step with `bullderdash_store_writes_total`.

Sizing guidance:

- Size the volume for the steady-state footprint: approximately
  `jobs_per_hour × STORE_COMPLETED_TTL_HOURS × bytes_per_indexed_job`, where
  `bytes_per_indexed_job` includes the FTS overhead (measure it on your own
  payloads; payload-heavy jobs can run ~1–2 KB/row or more).
- The largest lever on size is **what you index and how long you keep it**:
  lower `STORE_COMPLETED_TTL_HOURS` to shrink the dominant completed tier, and
  trim large fields from the job payload before they reach the store if full
  payloads are not needed for search.
- `VACUUM` is **not** recommended as routine maintenance. Because the store
  continuously churns (inserts ≈ deletes at steady state), freed pages are
  reused almost immediately, so a `VACUUM` reclaims little lasting space while
  taking an exclusive write lock (buffered writes drop during it) and needing
  free disk roughly equal to the database size. It is only worthwhile as a
  one-shot step *after* you reduce retention, run during a low-traffic window.
- Failures are retained longer (14 days by default) but are typically a tiny
  fraction of traffic, so the failed tier is usually a negligible contributor to
  total size.

Example p95 processing duration:
```promql
histogram_quantile(
  0.95,
  sum by (le, queue, name) (
    rate(bullmq_job_completion_duration_seconds_bucket[5m])
  )
)
```

Example completed/failed counts:
```promql
sum by (queue, name, result) (
  increase(bullmq_jobs_finished_total[5m])
)
```

## Simulator Notes

The simulator:
- Generates jobs at per-queue rates with occasional bursts
- Uses weighted job mixes per queue
- Injects delays, retries, and priorities
- Creates parent-child flows so `waiting-children` appears
- Occasionally pauses queues to exercise `paused`

See `scripts/sim/README.md` for details and knobs.

## Troubleshooting

### Queue stats show 0
1. Confirm Redis/Valkey is running (`PING` in `redis-cli`).
2. Confirm the simulator is running.
3. Verify `QUEUE_PREFIX` (default: `bull`).

### No queues found
- BullMQ only creates a queue after the first job is added. Start the simulator or your app.

### Jobs disappear quickly
- This is expected; jobs complete in seconds with auto-removal enabled.

## Commands

### Run Everything
```bash
# Terminal 1
./bullderdash.exe

# Terminal 2
cd scripts/sim
bun install
bun run index.ts

# Terminal 3
./redis-cli.exe
```

### Redis CLI
```redis
QUEUE-STATS orders
QUEUE-STATS emails
QUEUE-STATS billing
```

### Redis Key Examples
```redis
# List keys
KEYS bull:orders:*

# Waiting + active
LRANGE bull:orders:wait 0 -1
LRANGE bull:orders:active 0 -1

# Paused + waiting-children
LRANGE bull:orders:paused 0 -1
ZRANGE bull:orders:waiting-children 0 -1

# Prioritized + delayed + failed + completed + stalled
ZRANGE bull:orders:prioritized 0 -1
ZRANGE bull:orders:delayed 0 -1
ZRANGE bull:orders:failed 0 -1
ZRANGE bull:orders:completed 0 -1
ZRANGE bull:orders:stalled 0 -1

# Counts
LLEN bull:orders:wait
LLEN bull:orders:active
LLEN bull:orders:paused
ZCARD bull:orders:waiting-children
ZCARD bull:orders:prioritized
ZCARD bull:orders:delayed
ZCARD bull:orders:failed
ZCARD bull:orders:completed
ZCARD bull:orders:stalled
```
