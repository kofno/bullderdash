# Bull-der-dash Job Simulator

A realistic BullMQ job simulator that moves jobs through different states, simulating real-world processing scenarios.

## Features

✅ **Realistic Job Flow**: Jobs move through waiting → active → completed/failed states  
✅ **Multiple Job Types**: Different job types with varying success/failure rates  
✅ **Continuous Job Generation**: Adds new jobs every 2-4 seconds  
✅ **Simulated Processing**: Realistic processing times and progress updates  
✅ **Failures & Retries**: Some jobs fail and are retried automatically  
✅ **Delayed Jobs**: 20% of jobs are delayed to test the delayed state  
✅ **Backoff Strategy**: Failed jobs retry with exponential backoff  
✅ **Live Updates**: Watch queue states change in real-time on the dashboard  

## Quick Start

### Prerequisites
- Node.js 18+ or Bun
- Valkey/Redis running on `127.0.0.1:6379`
- Bull-der-dash running on `http://localhost:8080`

### Run the Simulator

```bash
cd scripts/sim

# Install dependencies
bun install

# Run the simulator
bun run index.ts
```

You'll see output showing jobs being added and processed!

## Configuration

### Environment Variables

```bash
# Specify which queues to create (default: orders,emails,billing)
QUEUES=orders,emails,billing,payments bun run index.ts
```

## Job Types & Success Rates

| Job Type | Fail Rate | Avg Time | Use Case |
|----------|-----------|----------|----------|
| `process-data` | 10% | 1000ms | Data processing |
| `send-email` | 20% | 500ms | Email delivery |
| `webhook-call` | 30% | 800ms | External API calls |
| `database-sync` | 5% | 2000ms | Database operations |

## Job States Generated

The simulator creates jobs in **all BullMQ states**:

### 🕐 Waiting
Jobs queued and waiting for a worker to pick them up

### 🚀 Active
Jobs currently being processed by a worker with progress updates

### ✅ Completed
Successfully processed jobs that accumulate over time

### ❌ Failed
Jobs that encountered errors after max retries

### ⏰ Delayed
Jobs scheduled for future processing (20% of new jobs)

## Real-Time Testing

### Setup

**Terminal 1** - Start Valkey:
```bash
docker run -d --name valkey -p 6379:6379 valkey/valkey:latest
```

**Terminal 2** - Start Bull-der-dash:
```bash
cd C:\RootDev\bull-der-dash
.\bullderdash.exe
```

**Terminal 3** - Start Simulator:
```bash
cd scripts/sim
bun run index.ts
```

**Browser** - Monitor live:
```
http://localhost:8080
```

### What to Watch For

1. **Queue depths** - Waiting/Active/Completed counts change in real-time
2. **Job flows** - Click stats to see jobs moving through states
3. **Progress updates** - Watch active jobs show progress
4. **Retries** - Failed jobs automatically retry with backoff
5. **Delayed jobs** - Watch them move from delayed to waiting

## Customization

### Increase Job Frequency (High Load Test)

Edit `index.ts`:

```typescript
setInterval(addJobs, 500 + Math.random() * 500);  // Faster job generation
```

### Change Failure Rates (Test Scenarios)

```typescript
const jobTypes = [
  { name: 'process-data', failRate: 1.0, delayMs: 1000 },  // Always fails
  { name: 'send-email', failRate: 0.0, delayMs: 500 },      // Never fails
];
```

### Change Worker Concurrency

```typescript
concurrency: 10,  // Process more jobs in parallel (default: 3)
```

## Stopping

Press `Ctrl+C` to gracefully shutdown.

## Troubleshooting

**Cannot connect to Valkey**: Make sure Redis is running on `127.0.0.1:6379`  
**No jobs in dashboard**: Refresh page, verify simulator is running  
**Jobs not completing**: Check "Worker started" messages in output

---

**Happy simulating!** 🎢

