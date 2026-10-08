# Bull-der-dash Architecture

## System Overview

```
┌─────────────────────────────────────────────────────────────────────┐
│                         Bull-der-dash                                │
│                     (Go HTTP Server - Port 8080)                     │
└─────────────────────────────────────────────────────────────────────┘
                                  │
              ┌───────────────────┼───────────────────┐
              │                   │                   │
              ▼                   ▼                   ▼
    ┌─────────────────┐  ┌─────────────────┐  ┌─────────────────┐
    │   Web Routes    │  │  Metrics API    │  │  Health Checks  │
    │ (HTMX UI +      │  │  (Prometheus)   │  │  (K8s Probes)   │
    │  Search Console)│  │                 │  │                 │
    └─────────────────┘  └─────────────────┘  └─────────────────┘
              │                   │                   │
              └───────────────────┼───────────────────┘
                                  │
                                  ▼
                    ┌──────────────────────────┐
                    │   Explorer Package       │
                    │  (Redis Client Logic)    │
                    │                          │
                    │  • DiscoverQueues()      │
                    │  • GetQueueStats()       │
                    │  • GetJob()              │
                    │  • GetJobsByState()      │
                    │  • determineJobState()   │
                    └──────────────────────────┘
                                  │
                                  │ Redis Protocol
                                  │
                                  ▼
                    ┌──────────────────────────┐
                    │    Redis / Valkey        │
                    │                          │
                    │  BullMQ Data Structures: │
                    │  • bull:{queue}:id       │
                    │  • bull:{queue}:wait     │
                    │  • bull:{queue}:active   │
                    │  • bull:{queue}:paused   │
                    │  • bull:{queue}:prioritized │
                    │  • bull:{queue}:waiting-children │
                    │  • bull:{queue}:failed   │
                    │  • bull:{queue}:completed│
                    │  • bull:{queue}:delayed  │
                    │  • bull:{queue}:stalled  │
                    │  • bull:{queue}:{jobId}  │
                    │  • bull:{queue}:events   │
                    └──────────────────────────┘
```

### History & Search Path (when `STORE_ENABLED=true`)

Search is decoupled from the live Redis path. A background collector tails the
BullMQ event streams and hands terminal (completed/failed) jobs to a background
writer that persists them to an embedded SQLite database. The search console and
JSON search API read exclusively from SQLite, so search never scans live Redis.

```
Redis BullMQ event streams
   │  (background tail)
   ▼
Workloadmetrics Collector ──> Prometheus metrics (in-memory)
   │  terminal jobs
   ▼
Store Writer (batched) ──> Embedded SQLite (WAL) ──> FTS5 full-text index
                                   ▲                        │
         Sweeper (tiered TTL) ─────┘           /console, /v1/search, /v1/jobs/{id}
         failures kept longer than successes            (read-only)
```

## Request Flow

### Dashboard Request
```
Browser
   │ GET /
   ▼
Main Handler (main.go)
   │ Returns HTML with HTMX
   ▼
Browser (HTMX)
   │ GET /queues (every 5s)
   ▼
DashboardHandler (web/handlers.go)
   │ Calls explorer.DiscoverQueues()
   │ Calls explorer.GetQueueStats()
   ▼
Explorer (explorer/explorer.go)
   │ Redis SCAN for bull:*:id
   │ Redis commands (LLEN, ZCARD)
   │ Updates Prometheus metrics
   ▼
HTML Table Response
   │ Rendered via template
   ▼
Browser (HTMX swaps content)
```

### Job List Request
```
Browser
   │ Click on "5 Failed" link
   │ GET /queue/jobs?queue=email&state=failed
   ▼
JobListHandler (web/handlers.go)
   │ Calls explorer.GetJobsByState()
   ▼
Explorer (explorer/explorer.go)
   │ Redis ZRANGE bull:email:failed
   │ For each jobID:
   │   Redis HGETALL bull:email:{jobId}
   │   Parse JSON fields
   ▼
HTML Table Response
   │ Job list with details
   ▼
Browser displays job list
```

### Job Detail Request
```
Browser
   │ Click "View Details →"
   │ GET /job/detail?queue=email&id=12345
   ▼
JobDetailHandler (web/handlers.go)
   │ Calls explorer.GetJob()
   ▼
Explorer (explorer/explorer.go)
   │ Redis HGETALL bull:email:12345
   │ Parse all job fields:
   │   • name, data, opts
   │   • progress, attempts
   │   • timestamps, stacktrace
   │ Determine state via multiple checks
   ▼
JSON Response
   │ Complete job object
   ▼
Browser displays JSON (or future HTML template)
```

### Metrics Request
```
Prometheus Scraper
   │ GET /metrics
   ▼
promhttp.Handler()
   │ Collects all registered metrics
   ▼
Text Response (Prometheus format)
   │ bullmq_queue_waiting{queue="email"} 42
   │ bullmq_queue_active{queue="email"} 5
   │ http_request_duration_seconds_bucket{...} 145
   │ (HTTP path labels are normalized to stable routes)
   │ redis_operation_duration_seconds_bucket{...} 89
   ▼
Prometheus stores & graphs
```

### Search Console Request (when `STORE_ENABLED=true`)
```
Browser
   │ GET /console
   ▼
ConsoleHandler (web/console.go)
   │ Serves embedded console.html (vanilla JS, no framework)
   ▼
Browser (fetch, same-origin)
   │ GET /v1/search?q=...&state=...&trace_id=...
   ▼
SearchAPI (web) ──> Store.Search() (internal/store)
   │ SQLite FTS5 query (bounded read concurrency)
   ▼
JSON rows ──> table render; click a row
   │ GET /v1/jobs/{id}
   ▼
Store.Job() ──> SQLite point lookup ──> JSON detail (data + opts)
   ▼
Browser renders detail overlay / trace lineage
```

## Data Flow

### Queue Discovery
```
Redis Keys:                    Explorer:                    Metrics:
bull:email:id       ─────>    DiscoverQueues()   ─────>   (none)
bull:sms:id                   • SCAN bull:*:id
bull:webhook:id               • Extract queue names
                              • Return ["email", "sms", "webhook"]
```

### Queue Statistics
```
Redis Commands:                    Explorer:                    Metrics:
LLEN bull:email:wait       ─┐
LLEN bull:email:active      │
LLEN bull:email:paused      │
ZCARD bull:email:prioritized│    GetQueueStats()
ZCARD bull:email:waiting-children│
ZCARD bull:email:failed     ├─>  • Execute commands          ─> QueueWaiting.Set()
ZCARD bull:email:completed  │    (individual)                  QueueActive.Set()
ZCARD bull:email:delayed    │    • Parse results               QueueFailed.Set()
ZCARD bull:email:stalled   ─┘    • Update metrics               QueueCompleted.Set()
                                   • Return QueueStats[]        QueueDelayed.Set()
```

### Job Retrieval
```
Redis Hash:                    Explorer:                    Result:
HGETALL bull:email:12345  ─>  GetJob()
                               • Parse JSON fields    ─>   Job struct:
Key-Value pairs:               • Unmarshal data              - ID: "12345"
  name: "send-email"           • Parse timestamps            - Name: "send-email"
  data: "{...}"                • Determine state             - Data: {...}
  opts: "{...}"                                              - State: "failed"
  timestamp: "1234567890"                                    - AttemptsMade: 3
  attemptsMade: "3"                                          - FailedReason: "..."
```

## Component Dependencies

```
main.go
  ├─> config/config.go (environment vars)
  ├─> explorer/explorer.go (Redis operations)
  │     └─> metrics/metrics.go (Prometheus)
  ├─> workloadmetrics/ (BullMQ event-stream collector)
  │     ├─> metrics/metrics.go (Prometheus)
  │     └─> store/store.go (persist terminal jobs)
  ├─> store/store.go (embedded SQLite history + FTS5; writer + sweeper)
  ├─> web/handlers.go (HTTP handlers)
  │     ├─> explorer/explorer.go
  │     └─> metrics/metrics.go
  ├─> web/console.go (embedded search console + /v1 search API)
  │     └─> store/store.go (read-only search/detail)
  └─> prometheus/promhttp (metrics endpoint)
```

## Configuration Flow

```
Environment Variables          Config Loader              Application
┌─────────────────┐          ┌──────────────┐          ┌─────────────┐
│ REDIS_ADDR      │   ─────> │ config.Load()│   ─────> │ Redis Client│
│ REDIS_PASSWORD  │          │              │          │ HTTP Server │
│ REDIS_DB        │          │ • getEnv()   │          │ Explorer    │
│ SERVER_PORT     │          │ • getEnvInt()│          └─────────────┘
│ QUEUE_PREFIX    │          │ • Defaults   │
│ LOG_LEVEL       │          └──────────────┘
└─────────────────┘
```

## Redis Connectivity Modes

Bull-der-dash supports two connection modes:

1. Direct mode
- Active when `REDIS_SENTINEL_MASTER` or `REDIS_SENTINEL_ADDRS` are not set.
- Uses `REDIS_ADDR` to connect with optional `REDIS_USERNAME` and `REDIS_PASSWORD`.

2. Sentinel mode
- Active when both `REDIS_SENTINEL_MASTER` and `REDIS_SENTINEL_ADDRS` are set.
- Uses go-redis failover client against Sentinel addresses.
- Supports:
`REDIS_SENTINEL_MASTER`, `REDIS_SENTINEL_ADDRS`, `REDIS_SENTINEL_USERNAME`, `REDIS_SENTINEL_PASSWORD`, `REDIS_USERNAME`, `REDIS_PASSWORD`, `REDIS_DB`.

## Concurrency Model

```
Main Goroutine
  │
  ├─> HTTP Server Goroutine
  │     │
  │     ├─> Request Handler 1 (goroutine per request)
  │     ├─> Request Handler 2
  │     ├─> Request Handler 3
  │     └─> ...
  │
  └─> Signal Handler Goroutine
        │ Waits for SIGINT/SIGTERM
        └─> Triggers graceful shutdown
```

## Error Handling Strategy

```
Layer                Error Handling
────────────────     ───────────────────────────────────
HTTP Handler         • http.Error() for user-facing errors
                     • Log internal errors
                     • Return 500 for unexpected errors

Explorer             • Return error to caller
                     • Increment error metrics
                     • Log with context

Redis Client         • go-redis automatic retries
                     • Connection pooling
                     • Error propagation
```

## State Management

Bull-der-dash keeps HTTP request handling stateless:
- Dashboard and inspection routes do not depend on session state
- Health checks stay cheap and do not depend on background collectors
- Queue depth metrics are refreshed in the background and exported from memory
- Job inspection data is fetched from Redis on demand

Optional background collectors may maintain small telemetry state when the data
being collected is event-derived. For example, workload metrics can keep
in-memory BullMQ event stream offsets, event lag, and bounded job-name
cardinality state. This telemetry state must not affect dashboard correctness,
health checks, or job inspection behavior.

In the current single-instance deployment model, a background collector can run
inside the bull-der-dash process. If the app is scaled horizontally later, any
global event-derived collector must either run in only one replica or add
explicit ownership coordination to avoid double-counting metrics.

When history persistence is enabled (`STORE_ENABLED=true`), the process also owns
an embedded SQLite database (WAL mode). SQLite is a single writer, so this mode
is single-replica by design: the Helm chart provisions one `ReadWriteOnce` PVC
and uses the `Recreate` update strategy so a rollout never leaves two pods
contending for the same volume. Horizontal scaling is only safe with history
persistence disabled (or with a future shared/external store).

## Performance Characteristics

| Operation | Latency | Notes |
|-----------|---------|-------|
| Queue discovery | ~10-20ms | SCAN command, cached in Redis |
| Queue stats | ~5-10ms | Multiple Redis commands |
| Job retrieval | ~2-5ms | Single HGETALL |
| Job list (100) | ~50-100ms | Multiple HGETALL calls |
| Metrics export | ~1-2ms | In-memory registry |
| Health check | ~1-2ms | Redis PING |

## Scaling Considerations

### Horizontal Scaling

Horizontal scaling is only safe with history persistence **disabled**
(`STORE_ENABLED=false`), since stateless read paths can sit behind a load
balancer. With persistence enabled, SQLite's single-writer model requires a
single replica (one RWO PVC + `Recreate` strategy); scale vertically instead.

```
┌─────────────┐     ┌─────────────┐     ┌─────────────┐
│ Bull-der-   │     │ Bull-der-   │     │ Bull-der-   │
│ dash Pod 1  │     │ dash Pod 2  │     │ dash Pod 3  │  (STORE_ENABLED=false)
└─────────────┘     └─────────────┘     └─────────────┘
      │                   │                   │
      └───────────────────┼───────────────────┘
                          │
                   ┌──────▼──────┐
                   │ Load Balancer│
                   └──────┬──────┘
                          │
                   ┌──────▼──────┐
                   │    Redis    │
                   └─────────────┘
```

### Resource Usage (per instance)
- **Memory**: 20-30 MB (higher with a large SQLite page cache under active search)
- **CPU**: 0.1 cores (idle), 0.5 cores (active)
- **Network**: Minimal (Redis protocol is efficient)
- **Disk**: 20 MB (binary only); plus the SQLite history DB on the mounted volume when persistence is enabled

### Bottlenecks
1. **Redis connection limit** - Use connection pooling (already implemented)
2. **Job list size** - Paginate for large queues (TODO)
3. **Metrics cardinality** - Queue name is only label (safe)
4. **SQLite single writer** - History persistence is single-replica; bound read concurrency with `STORE_READ_CONCURRENCY`


