# Quick Start Guide

## Get Bull-der-dash Running in 5 Minutes

### Step 1: Start Valkey (Redis)

If using kinD (local Kubernetes):
```bash
task kind:up
task valkey:up
```

Or use Docker directly:
```bash
docker run -d --name valkey -p 6379:6379 valkey/valkey:latest
```

Or use local Redis:
```bash
redis-server
```

### Step 2: Generate Test Data (Optional)

```bash
cd scripts/sim
bun install
bun run index.ts
```

This simulator continuously creates BullMQ jobs in Valkey that you can monitor. Jobs take 2-5 seconds to process, with realistic failure rates (10-35%).

### Step 3: Run Bull-der-dash

```bash
# Build
go build -o bullderdash.exe .

# Run with defaults (connects to localhost:6379)
./bullderdash.exe

# Or with custom configuration
REDIS_ADDR=127.0.0.1:6379 \
QUEUE_PREFIX=bull \
SERVER_PORT=8080 \
./bullderdash.exe
```

### Step 4: Open Dashboard

Visit: http://localhost:8080

You should see:
- 🐂 **Bull-der-dash Explorer** header
- Live queue statistics (auto-refresh every 5s)
- Links to 📊 Metrics and 💚 Health

### Step 5: Explore Features

#### View Queue Stats
The main dashboard shows all queues with:
- Waiting jobs (yellow)
- Active jobs (blue)
- Completed jobs (green)
- Failed jobs (red)
- Delayed jobs (purple)

Click any number to see the job list!

#### Check Metrics
Visit http://localhost:8080/metrics

You'll see Prometheus metrics for:
- Queue depths
- HTTP request timing
- Redis operation latency

#### View Job Details
1. Click on a queue stat (e.g., "5 Failed")
2. See the list of jobs
3. Click "View Details →" on any job
4. See full job data (JSON)

#### Test Health Checks
```bash
curl http://localhost:8080/health   # Should return: OK
curl http://localhost:8080/ready    # Should return: Ready
```

## Troubleshooting

### "Failed to connect to Redis"
- Ensure Redis/Valkey is running: `redis-cli ping` should return `PONG`
- Check `REDIS_ADDR` environment variable
- Verify port 6379 is accessible

### "No queues found"
- BullMQ hasn't created any queues yet
- Run the simulator to generate test jobs
- Check the queue prefix matches (default: `bull`)

### Build errors
```bash
# Clean and rebuild
go clean
go mod tidy
go build -v .
```

## What's Running?

| Component | Port | URL |
|-----------|------|-----|
| Bull-der-dash | 8080 | http://localhost:8080 |
| Valkey/Redis | 6379 | redis://localhost:6379 |
| Metrics | 8080 | http://localhost:8080/metrics |

## Environment Variables Cheat Sheet

```bash
# Minimal setup
export REDIS_ADDR=127.0.0.1:6379

# Full configuration
export REDIS_ADDR=127.0.0.1:6379
export REDIS_PASSWORD=mysecret
export REDIS_DB=0
export SERVER_PORT=8080
export QUEUE_PREFIX=bull
export LOG_LEVEL=info
```

## Docker Quick Start

```bash
# Build image
docker build -t bull-der-dash .

# Run container
docker run -d \
  --name bull-der-dash \
  -p 8080:8080 \
  -e REDIS_ADDR=host.docker.internal:6379 \
  bull-der-dash

# View logs
docker logs -f bull-der-dash
```

## Kubernetes Quick Start

```bash
# Apply deployment
kubectl apply -f k8s/deployment.yaml

# Check status
kubectl get pods -l app=bull-der-dash

# Port forward
kubectl port-forward svc/bull-der-dash 8080:80

# View logs
kubectl logs -l app=bull-der-dash -f
```

## Next Steps

1. ✅ Verify you see your queues
2. 📊 Set up Prometheus scraping (if using)
3. 🔍 Plan your search implementation (Bluge)
4. 🎨 Customize the UI (Tailwind classes in handlers.go)
5. 🚀 Add more features!

## Need Help?

Check these files:
- `README.md` - Full documentation
- `DOCS.md` - Complete feature guide
- `.env.example` - Configuration examples

Happy monitoring! 🎉

