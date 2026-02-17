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
export REDIS_PASSWORD=
export REDIS_DB=0
export SERVER_PORT=8080
export QUEUE_PREFIX=bull
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
| `REDIS_PASSWORD` | (empty) | Redis password if required |
| `REDIS_DB` | `0` | Redis database number |
| `SERVER_PORT` | `8080` | HTTP server port |
| `QUEUE_PREFIX` | `bull` | BullMQ queue prefix in Redis |
| `METRICS_POLL_SECONDS` | `10` | Background queue stats refresh interval (seconds) |
| `LOG_LEVEL` | `info` | Log level (debug, info, warn, error) |

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

## Kubernetes Deployment 🚢

Example deployment manifest:

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: bull-der-dash
spec:
  replicas: 2
  selector:
    matchLabels:
      app: bull-der-dash
  template:
    metadata:
      labels:
        app: bull-der-dash
    spec:
      containers:
      - name: bull-der-dash
        image: your-registry/bull-der-dash:latest
        ports:
        - containerPort: 8080
          name: http
        env:
        - name: REDIS_ADDR
          value: "valkey-service:6379"
        - name: QUEUE_PREFIX
          value: "bull"
        resources:
          requests:
            memory: "64Mi"
            cpu: "100m"
          limits:
            memory: "128Mi"
            cpu: "200m"
        livenessProbe:
          httpGet:
            path: /healthz
            port: 8080
          initialDelaySeconds: 10
          periodSeconds: 10
        readinessProbe:
          httpGet:
            path: /readyz
            port: 8080
          initialDelaySeconds: 5
          periodSeconds: 5
---
apiVersion: v1
kind: Service
metadata:
  name: bull-der-dash
spec:
  selector:
    app: bull-der-dash
  ports:
  - port: 80
    targetPort: 8080
  type: LoadBalancer
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
- `bull:{queue}:waiting-children` - List of parent jobs waiting on children
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
- **Scalable**: Stateless design allows horizontal scaling

## Contributing 🤝

Contributions welcome! Areas of focus:

1. **Search Implementation**: Bluge integration for job search
2. **Actions**: Porting BullMQ Lua scripts for job manipulation
3. **UI Polish**: Better visualizations and user experience
4. **Testing**: Unit and integration tests
5. **Documentation**: Expanded guides and examples

## License

[Add your license here]

## Acknowledgments

- [BullMQ](https://github.com/taskforcesh/bullmq) - The excellent Node.js queue library we're monitoring
- [Bluge](https://github.com/blugelabs/bluge) - Planned search engine integration
- [HTMX](https://htmx.org/) - Keeping the frontend simple and fast
