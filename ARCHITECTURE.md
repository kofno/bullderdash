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
    │   (HTMX UI)     │  │  (Prometheus)   │  │  (K8s Probes)   │
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
                    └──────────────────────────┘
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
   │ bullmq_queue_waiting_total{queue="email"} 42
   │ bullmq_queue_active_total{queue="email"} 5
   │ http_request_duration_seconds_bucket{...} 145
   │ (HTTP path labels are normalized to stable routes)
   │ redis_operation_duration_seconds_bucket{...} 89
   ▼
Prometheus stores & graphs
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
LLEN bull:email:waiting-children│
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
  ├─> web/handlers.go (HTTP handlers)
  │     ├─> explorer/explorer.go
  │     └─> metrics/metrics.go
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

Bull-der-dash is **completely stateless**:
- No in-memory caches
- No session storage
- All data fetched from Redis on-demand
- Can scale horizontally without coordination

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
```
┌─────────────┐     ┌─────────────┐     ┌─────────────┐
│ Bull-der-   │     │ Bull-der-   │     │ Bull-der-   │
│ dash Pod 1  │     │ dash Pod 2  │     │ dash Pod 3  │
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
- **Memory**: 20-30 MB
- **CPU**: 0.1 cores (idle), 0.5 cores (active)
- **Network**: Minimal (Redis protocol is efficient)
- **Disk**: 20 MB (binary only)

### Bottlenecks
1. **Redis connection limit** - Use connection pooling (already implemented)
2. **Job list size** - Paginate for large queues (TODO)
3. **Metrics cardinality** - Queue name is only label (safe)


