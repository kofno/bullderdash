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

## Endpoints

- `GET /` - Dashboard
- `GET /queues` - HTMX queue list fragment
- `GET /queue/<name>` - Queue detail view
- `GET /queue/jobs?queue=<name>&state=<state>` - State job list
- `GET /job/detail?queue=<name>&id=<id>` - Job detail (JSON)
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
- `LOG_LEVEL` (default `info`)

Workload metrics are collected from BullMQ event streams in a background
goroutine. `/metrics` only exports in-memory Prometheus data; it does not scan
retained jobs or issue Redis commands during a scrape.

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
