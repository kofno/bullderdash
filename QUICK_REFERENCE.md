# 📋 Quick Reference Card

## The Three Programs

```
bullderdash.exe    → Main dashboard    http://localhost:8080
redis-cli.exe      → Redis inspector   .\redis-cli.exe
bun run index.ts   → Job simulator     cd scripts\sim && bun run index.ts
```

## Dashboard Navigation

```
Home (http://localhost:8080)
  ↓
  ├─ Click queue name
  │  └─ Queue detail (/queue/orders)
  │     └─ Click "View →" on a job
  │        └─ Job details (JSON)
  │
  └─ Click "📊 Metrics"
     └─ Prometheus data
```

## Redis CLI Commands

```bash
.\redis-cli.exe
> HELP                          # Show all commands
> QUEUE-STATS orders            # Get queue stats
> LRANGE bull:orders:wait 0 10  # List waiting jobs
> HGETALL bull:orders:123       # Get job data
> KEYS bull:*                   # List all BullMQ keys
> DBSIZE                        # Total keys
> FLUSHDB                       # Clear all data
> QUIT                          # Exit
```

## Common Tasks

### Check Queue Status
```bash
.\redis-cli.exe
> QUEUE-STATS orders
```

### List All Jobs Waiting
```bash
.\redis-cli.exe
> LRANGE bull:orders:wait 0 100
```

### Get Specific Job Data
```bash
.\redis-cli.exe
> HGETALL bull:orders:1
```

### See Everything in Redis
```bash
.\redis-cli.exe
> KEYS bull:*
```

### Reset Everything
```bash
.\redis-cli.exe
> FLUSHDB
# Then restart simulator
```

## The UI Views

### Dashboard
- **URL**: http://localhost:8080
- **Shows**: All queues with stats
- **Click**: Queue name or "View →"

### Queue Detail
- **URL**: http://localhost:8080/queue/orders
- **Shows**: Single queue with all jobs by state
- **Click**: Job "View →" for details

### Job Detail
- **URL**: http://localhost:8080/job/detail?queue=orders&id=123
- **Shows**: Complete job data (JSON)
- **Data**: All job fields, errors, progress, etc.

## Starting Everything

```bash
# Terminal 1: Database
docker run -d --name valkey -p 6379:6379 valkey/valkey:latest

# Terminal 2: Job Generator
cd scripts\sim && bun run index.ts

# Terminal 3: Dashboard
.\bullderdash.exe

# Terminal 4: Inspector (as needed)
.\redis-cli.exe
```

## Metrics

**URL**: http://localhost:8080/metrics

**Shows**:
- Queue depths (waiting, active, etc.)
- HTTP request latency
- Redis operation timing

## Health Checks

```bash
curl http://localhost:8080/health    # Liveness
curl http://localhost:8080/ready     # Readiness
```

## Environment Variables

```bash
REDIS_ADDR=127.0.0.1:6379
REDIS_PASSWORD=
REDIS_DB=0
SERVER_PORT=8080
QUEUE_PREFIX=bull
LOG_LEVEL=info
```

## File Locations

```
C:\RootDev\bull-der-dash\
├── bullderdash.exe               ← Dashboard binary
├── redis-cli.exe                 ← CLI tool
├── scripts\sim\index.ts          ← Simulator
└── internal\
    ├── explorer\                 ← Redis queries
    ├── web\                      ← HTTP handlers
    ├── config\                   ← Configuration
    └── metrics\                  ← Prometheus
```

## Troubleshooting

| Problem | Solution |
|---------|----------|
| Dashboard won't start | Check Valkey is running: `docker ps` |
| No queues showing | Run simulator: `bun run scripts/sim/index.ts` |
| Zero counts everywhere | Use CLI: `.\redis-cli.exe > QUEUE-STATS orders` |
| Count mismatches | Normal! 5s polling = race conditions |
| Can't connect to Redis | Check `127.0.0.1:6379` is accessible |

## Quick Test

```bash
# 1. Start Valkey
docker run -d --name valkey -p 6379:6379 valkey/valkey:latest

# 2. Start simulator
cd scripts\sim && bun run index.ts

# 3. Start dashboard  
.\bullderdash.exe

# 4. Open browser
http://localhost:8080

# 5. Try CLI
.\redis-cli.exe
> QUEUE-STATS orders
```

## What Each Color Means

| Color | Meaning |
|-------|---------|
| 🕐 Yellow | Waiting for processing |
| 🚀 Blue | Currently processing |
| 🌿 Green | Completed successfully |
| 🔴 Red | Failed (max retries) |
| 🟣 Purple | Delayed (scheduled) |

---

**Bookmark this for quick reference!** 📍

