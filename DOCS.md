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
- Available when `STORE_ENABLED=true`
- Full-text search over persisted job history (name, trace id, last error, payloads)
- Filter by queue, job name, state, trace id, and time window
- Drill down into a trace's lineage and open full single-job detail
- Dependency-free page served from the embedded SQLite FTS5 store — it never scans live Redis

## Endpoints

- `GET /` - Dashboard
- `GET /queues` - HTMX queue list fragment
- `GET /queue/<name>` - Queue detail view
- `GET /queue/jobs?queue=<name>&state=<state>` - State job list
- `GET /job/detail?queue=<name>&id=<id>` - Job detail (JSON)
- `GET /console` - Full-text search console (when `STORE_ENABLED=true`)
- `GET /v1/search?q=&name=&state=&trace_id=&since_ms=&limit=` - Search persisted history (JSON; requires at least one of `q`/`name`/`state`/`trace_id`, else `400`; `503` when readers are saturated)
- `GET /v1/jobs/{id}` - Persisted detail for one job (JSON; `404` once it has aged out of retention)
- `GET /metrics` - Prometheus metrics
- `GET /health` and `GET /ready` - Health checks

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
- `STORE_ENABLED` (default `false`) - persist completed/failed jobs to SQLite and enable `/console`
- `STORE_DB_PATH` (default `/data/history.db`) - SQLite file path; its directory must be writable
- `STORE_WRITE_BUFFER` (default `4096`) - in-memory write channel size; records drop when full
- `STORE_BATCH_SIZE` (default `256`) - max records per write transaction
- `STORE_FLUSH_MILLIS` (default `500`) - max time a batch waits before flushing
- `STORE_TRACE_KEYS` (default empty) - comma-separated payload keys used to derive a trace id
- `STORE_COMPLETED_TTL_HOURS` (default `24`) - retention for non-failed history; `0` disables
- `STORE_FAILED_TTL_HOURS` (default `336`) - retention for failed history (14 days); `0` disables
- `STORE_SWEEP_SECONDS` (default `300`) - interval between retention sweeps
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
