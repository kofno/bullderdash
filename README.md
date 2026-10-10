# Bull-der-dash

A high-performance dashboard for monitoring BullMQ queues, built in Go for speed, efficiency, and Kubernetes-native deployments.

## Features ✨

### Current (MVP)
- **Live Queue Monitoring**: Real-time updates every 5 seconds
- **Multi-State Tracking**: waiting, active, paused, prioritized, waiting-children, completed, failed, delayed, stalled, orphaned
- **Queue Detail View**: Single-queue view with jobs grouped by state
- **Job Introspection**: JSON detail for any job
- **Persistent Job History**: Completed/failed jobs are recorded to an embedded
  SQLite database (WAL mode, pure-Go `modernc.org/sqlite`, no CGO) off the hot
  path, so the live Redis/Valkey instance is never scanned to answer queries
- **Full-Text Search Console**: `/console` search UI backed by SQLite FTS5 over
  job name, trace id, last error, and payloads — with trace-lineage drill-down
  and single-job detail, served as a dependency-free same-origin page
- **Tiered Retention Sweeper**: Background goroutine that ages out history, keeping
  failures longer than successes, with an optional hard row cap
- **Prometheus Metrics**: Built-in `/metrics` endpoint
- **Health Checks**: `/health` and `/ready`
- **Environment Configuration**: 12-factor app design with environment variables
- **Lightweight**: Low memory footprint and fast response times, single static binary

### Roadmap 🗺️
- **Actions**: Retry, remove, pause/resume operations (requires porting BullMQ Lua scripts)
- **Alerts**: Threshold-based notifications
- **Historical Metrics**: Time-series data and trends
- **Rate Limiting Visibility**: Show configured rates and throughput
- **Job Replaying**: Re-queue failed jobs
- **Bulk Operations**: Batch actions across multiple queues
- **Access Control**: RBAC for production safety

## Architecture 🏗️

```
bull-der-dash/
├── main.go                 # Application entry point
├── cmd/
│   └── redis-cli/           # Lightweight Redis/Valkey CLI tool
├── internal/
│   ├── config/            # Environment-based configuration
│   ├── explorer/          # Redis/Valkey interaction & BullMQ parsing
│   ├── metrics/           # Prometheus metrics definitions
│   ├── store/             # Embedded SQLite job-history store (FTS5 search)
│   ├── workloadmetrics/   # Event-stream collector + history persistence
│   └── web/               # HTTP handlers, templates & embedded search console
```

> The live dashboard is HTMX-driven, but the search console (`/console`) is a
> dependency-free, framework-free page (vanilla JS, embedded via `go:embed`) that
> talks only to the same-origin `/v1/search` and `/v1/jobs/{id}` JSON endpoints.
> New UI work targets the console; HTMX is being phased out.

## Quick Start 🚀

### Prerequisites
- Go 1.26+
- Redis/Valkey instance with BullMQ data
- Bun (for the simulator)
- (Optional) Kubernetes cluster for deployment
- (Optional) A writable volume for the SQLite history database when `STORE_ENABLED=true`

### Local Development

```bash
# Clone the repository
git clone <your-repo-url>
cd bull-der-dash

# Set environment variables (optional, defaults shown)
export REDIS_ADDR=127.0.0.1:6379
export REDIS_USERNAME=
export REDIS_PASSWORD=
export REDIS_DB=0
export REDIS_SENTINEL_MASTER=
export REDIS_SENTINEL_ADDRS=
export REDIS_SENTINEL_USERNAME=
export REDIS_SENTINEL_PASSWORD=
export SERVER_PORT=8080
export QUEUE_PREFIX=bull
export METRICS_POLL_SECONDS=10
export DASHBOARD_REFRESH_TIMEOUT_SECONDS=30
export WORKLOAD_METRICS_ENABLED=false
export WORKLOAD_METRICS_POLL_SECONDS=10
export WORKLOAD_METRICS_BLOCK_SECONDS=1
export WORKLOAD_METRICS_BATCH_SIZE=100
export WORKLOAD_METRICS_MAX_JOB_NAMES_PER_QUEUE=100
export WORKLOAD_METRICS_START_ID='$'
export STORE_ENABLED=false
export STORE_DB_PATH=/data/history.db
export STORE_WRITE_BUFFER=4096
export STORE_BATCH_SIZE=256
export STORE_FLUSH_MILLIS=500
export STORE_TRACE_KEYS=
export STORE_COMPLETED_TTL_HOURS=24
export STORE_FAILED_TTL_HOURS=336
export STORE_SWEEP_SECONDS=300
export STORE_MAX_ROWS=0
export STORE_READ_CONCURRENCY=16
export LOG_LEVEL=info

# Build and run
# Option A: Taskfile build (recommended)
task build
./bullderdash.exe

# Option B: Go build
go build -o bullderdash.exe .
./bullderdash.exe
```

Visit http://localhost:8080 to see your dashboard!

### Using with kinD (local K8s)

```bash
# Start local cluster with Valkey
task kind:up
task valkey:up

# Run the simulator to generate test jobs
cd scripts/sim
bun install
bun run index.ts

# Run bull-der-dash
./bullderdash.exe
```

> On Windows PowerShell, use `./bullderdash.exe` or `.\bullderdash.exe`

## Configuration ⚙️

All configuration is done via environment variables:

| Variable | Default | Description |
|----------|---------|-------------|
| `REDIS_ADDR` | `127.0.0.1:6379` | Redis/Valkey connection string |
| `REDIS_USERNAME` | (empty) | Redis username (ACL) |
| `REDIS_PASSWORD` | (empty) | Redis password if required |
| `REDIS_DB` | `0` | Redis database number |
| `REDIS_SENTINEL_MASTER` | (empty) | Sentinel master name; enables Sentinel mode when set with addrs |
| `REDIS_SENTINEL_ADDRS` | (empty) | Comma-separated Sentinel addresses (e.g. `10.0.0.1:26379,10.0.0.2:26379`) |
| `REDIS_SENTINEL_USERNAME` | (empty) | Sentinel username (if required) |
| `REDIS_SENTINEL_PASSWORD` | (empty) | Sentinel password (if required) |
| `SERVER_PORT` | `8080` | HTTP server port |
| `QUEUE_PREFIX` | `bull` | BullMQ queue prefix in Redis |
| `METRICS_POLL_SECONDS` | `10` | Background queue stats refresh interval (seconds) |
| `DASHBOARD_REFRESH_TIMEOUT_SECONDS` | `30` | Deadline for each dashboard snapshot refresh |
| `WORKLOAD_METRICS_ENABLED` | `false` | Enable event-stream workload metrics for completed/failed jobs |
| `WORKLOAD_METRICS_POLL_SECONDS` | `10` | Queue discovery interval for workload metrics |
| `WORKLOAD_METRICS_BLOCK_SECONDS` | `1` | Redis `XREAD` block timeout for workload metrics |
| `WORKLOAD_METRICS_BATCH_SIZE` | `100` | Maximum BullMQ event stream entries read per `XREAD` call |
| `WORKLOAD_METRICS_MAX_JOB_NAMES_PER_QUEUE` | `100` | Per-queue job-name label cardinality cap; additional names use `__other__` |
| `WORKLOAD_METRICS_START_ID` | `$` | Initial BullMQ event stream ID; `$` starts with new events only |
| `STORE_ENABLED` | `false` | Persist completed/failed jobs to the embedded SQLite history store (powers `/console` search) |
| `STORE_DB_PATH` | `/data/history.db` | Path to the SQLite database file; its directory must be writable (mount a volume) |
| `STORE_WRITE_BUFFER` | `4096` | Size of the in-memory write channel; records are dropped (and counted) when full |
| `STORE_BATCH_SIZE` | `256` | Max records flushed per write transaction |
| `STORE_FLUSH_MILLIS` | `500` | Max time a batch waits before being flushed |
| `STORE_TRACE_KEYS` | (empty) | Comma-separated payload keys to derive a trace id from; falls back to flow-root/own job id |
| `STORE_COMPLETED_TTL_HOURS` | `24` | Retention for non-failed history; `0` disables this tier |
| `STORE_FAILED_TTL_HOURS` | `336` | Retention for failed history (default 14 days); `0` disables this tier |
| `STORE_SWEEP_SECONDS` | `300` | Interval between retention sweeps |
| `STORE_MAX_ROWS` | `0` | Optional hard cap on total history rows (newest kept); `0` disables the cap |
| `STORE_READ_CONCURRENCY` | `16` | Max concurrent search/detail reads; excess requests get HTTP 503 |
| `LOG_LEVEL` | `info` | Log level (debug, info, warn, error) |

Sentinel behavior:
- If both `REDIS_SENTINEL_MASTER` and `REDIS_SENTINEL_ADDRS` are set, the app uses Redis Sentinel failover mode.
- Otherwise, the app uses direct `REDIS_ADDR` mode.

## Endpoints 🌐

### Web UI
- `GET /` - Main dashboard
- `GET /queues` - HTMX partial: queue list
- `GET /queue/<name>` - Single-queue detail view
- `GET /queue/jobs?queue=<name>&state=<state>` - Job list for a queue/state
- `GET /job/detail?queue=<name>&id=<id>` - Job detail (JSON)
- `GET /console` - Full-text search console (only when `STORE_ENABLED=true`)

### Search API (JSON)
Available when `STORE_ENABLED=true`; backs the `/console` UI and is safe to call directly.
- `GET /v1/search?q=&name=&state=&trace_id=&errored=&since_ms=&limit=` - Search persisted history (requires at least one of `q`/`name`/`state`/`trace_id`/`errored`/`since_ms`; returns `400` otherwise, `503` when the reader is saturated). `state` only matches the settled states history records (`Completed`/`Failed`). `since_ms` is a standalone "finished within" window (Unix ms lower bound on `finished_at`).
- `GET /v1/jobs/{id}` - Full persisted detail for one job (`404` if it has aged out of retention)
- `GET /v1/stats/job-names?queue=&state=&since_ms=&limit=` - Highest-volume job names from history, grouped by `(name, queue, state)` and ordered by count descending. All filters are optional (no predicate required). Unlike the Prometheus `bullmq_jobs_finished_total` labels — which cap distinct names per queue and collapse the overflow into an `__other__` bucket — this report uses the raw, uncapped job name recorded for every job, so it is exact and never emits `__other__`. Powers the Grafana "Top job names" table via the Infinity datasource. `queue` is treated as an exact match for a single name; empty, `All`, `.*`, or any value containing regex/alternation characters (e.g. a Grafana multi-select) is ignored so every queue is included.

> **Settled state only.** The history store captures *terminal* job events
> (`completed`/`failed`) — transient states (`active`/`waiting`/`delayed`) are
> intentionally not persisted, so the live queue is never slowed by history
> writes. Because BullMQ emits a `failed` event only once retries are
> exhausted, a job that errored on an attempt but ultimately succeeded is
> recorded as `Completed` while retaining its last error and true attempt
> count. Use `errored=true` (last error present **or** more than one attempt)
> to find those retried/troubled jobs regardless of final state.

### Operations
- `GET /health` or `/healthz` - Health check (liveness probe)
- `GET /ready` or `/readyz` - Readiness check (readiness probe)
- `GET /metrics` - Prometheus metrics

## Redis CLI Tool (Windows-friendly) 🧰

A lightweight Redis/Valkey CLI is included for Windows users (and works cross-platform).

### Build the CLI

```bash
# Option A: Taskfile build (recommended)
task build-cli

# Option B: Go build
cd cmd/redis-cli
go build -o ../../redis-cli.exe .
```

### Build both binaries

```bash
# Build dashboard + redis CLI
task build-all
```

### Use the CLI

```bash
# From repo root
./redis-cli.exe
```

Sentinel example:

```bash
./redis-cli.exe --sentinel-master mymaster --sentinel-addrs 10.0.0.1:26379,10.0.0.2:26379 --password your-redis-password
```

### Common Commands

```bash
> HELP
> QUEUE-STATS orders
> KEYS bull:*
> LRANGE bull:orders:wait 0 10
> HGETALL bull:orders:1
> TYPE bull:orders:wait
```

## Metrics 📊

Bull-der-dash exposes the following Prometheus metrics:

### Queue Metrics
- `bullmq_queue_waiting{queue="<name>"}` - Jobs waiting to be processed
- `bullmq_queue_active{queue="<name>"}` - Jobs currently processing
- `bullmq_queue_paused{queue="<name>"}` - Jobs paused
- `bullmq_queue_prioritized{queue="<name>"}` - Prioritized jobs
- `bullmq_queue_waiting_children{queue="<name>"}` - Jobs waiting on children
- `bullmq_queue_failed{queue="<name>"}` - Failed jobs
- `bullmq_queue_completed{queue="<name>"}` - Completed jobs
- `bullmq_queue_delayed{queue="<name>"}` - Delayed jobs
- `bullmq_queue_stalled{queue="<name>"}` - Stalled jobs
- `bullmq_queue_orphaned{queue="<name>"}` - Orphaned job hashes

### Performance Metrics
- `http_request_duration_seconds{method, path, status}` - HTTP request latency (path is normalized to stable routes)
- `redis_operation_duration_seconds{operation}` - Redis operation latency
- `redis_operation_errors_total{operation}` - Redis operation errors

### Workload Metrics
When `WORKLOAD_METRICS_ENABLED=true`, bull-der-dash reads BullMQ event streams
in a background goroutine and exports workload visibility without scanning
retained jobs during Prometheus scrapes.

- `bullmq_jobs_finished_total{queue, name, result}` - Observed completed/failed jobs
- `bullmq_job_completion_duration_seconds{queue, name, result}` - Histogram of `finishedOn - processedOn`
- `bullmq_workload_event_lag_seconds{queue}` - Approximate age of latest observed event stream entry
- `bullmq_workload_events_read_total{queue, event}` - Event stream entries read
- `bullmq_workload_events_dropped_total{queue, reason}` - Terminal events skipped because the event itself was missing required fields
- `bullmq_workload_job_lookup_errors_total{queue, reason}` - Job hash lookup or parsing failures

The `name` label is the BullMQ job name. To keep Prometheus cardinality bounded,
new job names are capped per queue by `WORKLOAD_METRICS_MAX_JOB_NAMES_PER_QUEUE`.
Additional names are reported as `__other__`. If a terminal event is observed
but the job hash is already gone, the count is still recorded with
`name="__unknown__"` and the duration sample is skipped.

The collector starts at `WORKLOAD_METRICS_START_ID`, which defaults to `$`.
That means it observes new BullMQ events after startup and does not backfill
already-retained completed or failed jobs.

#### Useful PromQL

Completed and failed jobs over the last 5 minutes:

```promql
sum by (queue, name, result) (
  increase(bullmq_jobs_finished_total[5m])
)
```

Jobs completed per second by queue and job name:

```promql
sum by (queue, name) (
  rate(bullmq_jobs_finished_total{result="completed"}[5m])
)
```

Failed jobs per second by queue and job name:

```promql
sum by (queue, name) (
  rate(bullmq_jobs_finished_total{result="failed"}[5m])
)
```

Failure ratio by queue and job name:

```promql
sum by (queue, name) (
  rate(bullmq_jobs_finished_total{result="failed"}[5m])
)
/
sum by (queue, name) (
  rate(bullmq_jobs_finished_total[5m])
)
```

p95 processing duration by queue and job name:

```promql
histogram_quantile(
  0.95,
  sum by (le, queue, name) (
    rate(bullmq_job_completion_duration_seconds_bucket[5m])
  )
)
```

p95 processing duration by queue:

```promql
histogram_quantile(
  0.95,
  sum by (le, queue) (
    rate(bullmq_job_completion_duration_seconds_bucket[5m])
  )
)
```

Average processing duration by queue and job name:

```promql
sum by (queue, name) (
  rate(bullmq_job_completion_duration_seconds_sum[5m])
)
/
sum by (queue, name) (
  rate(bullmq_job_completion_duration_seconds_count[5m])
)
```

Collector event lag:

```promql
bullmq_workload_event_lag_seconds
```

Collector lookup issues:

```promql
sum by (queue, reason) (
  increase(bullmq_workload_job_lookup_errors_total[15m])
)
```

Cardinality guardrail check:

```promql
sum by (queue, name) (
  increase(bullmq_jobs_finished_total{name=~"__other__|__unknown__"}[15m])
)
```

## Helm Install (GHCR) ⛵

Bull-der-dash is packaged as a Helm chart and published to GHCR as an OCI artifact.

```bash
# Log in to GHCR (GitHub token with packages:read)
echo $GITHUB_TOKEN | helm registry login ghcr.io -u kofno --password-stdin

# Install from OCI chart
helm install bull-der-dash oci://ghcr.io/kofno/charts/bull-der-dash \
  --version 0.2.0 \
  --namespace mynamespace \
  --create-namespace \
  --set image.repository=ghcr.io/kofno/bull-der-dash
```

Example values (Redis Sentinel):

```yaml
env:
  redis:
    sentinelMaster: "mymaster"
    sentinelAddrs: "10.0.0.1:26379,10.0.0.2:26379,10.0.0.3:26379"
    passwordSecret:
      name: redis-auth
      key: redis-password
```

### Persistent job history (SQLite)

To enable the `/console` search UI, turn on the store and give it a volume. Because
SQLite is a single writer, the chart provisions a single `ReadWriteOnce` PVC and
switches the Deployment to the `Recreate` strategy so a rollout never leaves two
pods contending for the same volume.

```yaml
env:
  store:
    enabled: true
    # dbPath lives under persistence.mountPath so it lands on the volume
    dbPath: /data/history.db
    failedTtlHours: 336   # keep failures 14 days
    completedTtlHours: 24 # keep successes 1 day
    # traceKeys: "traceId,trace_id"  # optional payload keys for trace lineage

persistence:
  enabled: true
  mountPath: /data
  size: 5Gi
  # storageClass: ""        # "" uses the cluster default; "-" forces no class
  # existingClaim: ""       # reuse a pre-created PVC instead of provisioning one
```

## Development 🛠️

### Project Structure

- **`internal/explorer`**: Handles all Redis/Valkey communication and BullMQ data structure parsing
- **`internal/web`**: HTTP handlers and HTML templates
- **`internal/metrics`**: Prometheus metric definitions
- **`internal/config`**: Configuration management

### Adding New Features

1. **New metrics**: Add to `internal/metrics/metrics.go`
2. **New endpoints**: Add handlers to `internal/web/handlers.go`
3. **New Redis queries**: Add methods to `internal/explorer/explorer.go`
4. **New search/history fields**: Add to the schema and queries in `internal/store/store.go`
5. **Console UI changes**: Edit `internal/web/assets/console.html` (embedded via `go:embed`)

### BullMQ Data Structures

BullMQ stores data in Redis with these key patterns:

- `bull:{queue}:id` - Queue ID counter
- `bull:{queue}:wait` - List of waiting job IDs
- `bull:{queue}:active` - List of active job IDs
- `bull:{queue}:paused` - List of paused job IDs
- `bull:{queue}:prioritized` - Sorted set of prioritized jobs (score = priority)
- `bull:{queue}:waiting-children` - Sorted set of parent jobs waiting on children
- `bull:{queue}:failed` - Sorted set of failed jobs (score = timestamp)
- `bull:{queue}:completed` - Sorted set of completed jobs (score = timestamp)
- `bull:{queue}:delayed` - Sorted set of delayed jobs (score = timestamp)
- `bull:{queue}:stalled` - Sorted set of stalled jobs (score = timestamp)
- `bull:{queue}:{jobId}` - Hash containing job data

## Performance 🚀

Bull-der-dash is designed for efficiency:

- **Low Memory**: ~20-30MB RSS under typical load
- **Fast Queries**: Lightweight per-key Redis commands for queue stats
- **Concurrent**: Go's goroutines handle multiple requests efficiently
- **Scalable**: HTTP request handling stays stateless for horizontal scaling
- **Workload Metrics**: Optional background event-stream collection avoids retained-job scans

## Contributing 🤝

Contributions welcome! Areas of focus:

1. **Actions**: Porting BullMQ Lua scripts for job manipulation
2. **UI Polish**: Better visualizations and search console UX
3. **Testing**: Unit and integration tests
4. **Documentation**: Expanded guides and examples

## License

MIT (see `LICENSE`)

## Acknowledgments

- [BullMQ](https://github.com/taskforcesh/bullmq) - The excellent Node.js queue library we're monitoring
- [modernc.org/sqlite](https://pkg.go.dev/modernc.org/sqlite) - Pure-Go, CGO-free SQLite (with FTS5) powering persistent history and search
