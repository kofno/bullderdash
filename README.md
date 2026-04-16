# Bull-der-dash

A high-performance dashboard for monitoring BullMQ queues, built in Go for speed, efficiency, and Kubernetes-native deployments.

## Features ✨

### Current (MVP)
- **Live Queue Monitoring**: Real-time updates every 5 seconds
- **Multi-State Tracking**: waiting, active, paused, prioritized, waiting-children, completed, failed, delayed, stalled, orphaned
- **Queue Detail View**: Single-queue view with jobs grouped by state
- **Job Introspection**: JSON detail for any job
- **Prometheus Metrics**: Built-in `/metrics` endpoint
- **Health Checks**: `/health` and `/ready`
- **Environment Configuration**: 12-factor app design with environment variables
- **Lightweight**: Low memory footprint and fast response times
- **HTMX-powered UI**: Interactive dashboard without heavy JavaScript frameworks

### Roadmap 🗺️
- **Search**: Bluge-powered full-text search across job data
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
│   └── web/               # HTTP handlers & templates
```

## Quick Start 🚀

### Prerequisites
- Go 1.25.4+
- Redis/Valkey instance with BullMQ data
- Bun (for the simulator)
- (Optional) Kubernetes cluster for deployment

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
  --version 0.0.2 \
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

1. **Search Implementation**: Bluge integration for job search
2. **Actions**: Porting BullMQ Lua scripts for job manipulation
3. **UI Polish**: Better visualizations and user experience
4. **Testing**: Unit and integration tests
5. **Documentation**: Expanded guides and examples

## License

MIT (see `LICENSE`)

## Acknowledgments

- [BullMQ](https://github.com/taskforcesh/bullmq) - The excellent Node.js queue library we're monitoring
- [Bluge](https://github.com/blugelabs/bluge) - Planned search engine integration
- [HTMX](https://htmx.org/) - Keeping the frontend simple and fast
