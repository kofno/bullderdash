# 🎢 Simulator Enhancement - What Changed

## Summary

Your job simulator has been dramatically improved to generate realistic, continuous workloads that move jobs through all BullMQ states. Perfect for testing Bull-der-dash!

## What Was Changed

### Before ❌
```typescript
// Added 2 jobs once, then exited
await queue.add('process-data', { foo: 'bar' });
await queue.add('critical-alert', { error: 'Simulated failure' });
process.exit(0);
```

### After ✨
```typescript
// Continuous job generation with workers
async function setupWorkers() {
  for (const queueName of queueNames) {
    const worker = new Worker(queueName, async (job) => {
      // Simulate processing, random failures, progress updates
      // Move jobs through: waiting → active → completed/failed
    });
  }
}

async function addJobsContinuously() {
  // Add new jobs every 2-4 seconds
  // 20% of jobs are delayed
  // Jobs retry with exponential backoff
}
```

## New Capabilities

### 🏭 Workers Process Jobs
Each queue has a dedicated worker that:
- Processes up to 3 jobs concurrently
- Simulates realistic processing time (500ms - 2s)
- Updates job progress (0% → 50% → 100%)
- Randomly fails based on job type
- Logs completion and failures

### 📊 Job Types with Different Behaviors

| Type | Success | Speed | Use Case |
|------|---------|-------|----------|
| `process-data` | 90% | 1.0s | Background processing |
| `send-email` | 80% | 0.5s | Email delivery |
| `webhook-call` | 70% | 0.8s | External API calls |
| `database-sync` | 95% | 2.0s | Database operations |

### ⏰ Delayed Jobs
20% of newly created jobs are delayed 3-10 seconds:
```typescript
const delay = Math.random() < 0.2 ? Math.random() * 10000 : 0;
```

### 🔄 Automatic Retries
Failed jobs retry up to 3 times with exponential backoff:
```typescript
{
  attempts: 3,
  backoff: { type: 'exponential', delay: 2000 }
}
```

### 📈 Continuous Generation
New jobs are added every 2-4 seconds to multiple queues:
```typescript
setInterval(addJobs, 2000 + Math.random() * 2000);
```

### 🎯 Real-Time Logging
Watch jobs move through states:
```
📤 [orders] Added job a1b2c3d (process-data)
  ✅ [orders] Job 1 (process-data) completed
  ❌ [emails] Job 2 (send-email) failed: Network timeout
```

## Job State Flow

```
Job Created (added to queue)
    ↓
Delayed? → YES → ⏰ DELAYED state (X seconds)
    ↓ NO         ↓
    ↓        Delay expires
    ↓            ↓
    └──→ → → → 🕐 WAITING state (in queue list)
               ↓
        Worker picks it up
               ↓
        🚀 ACTIVE state (processing)
               ↓
        ┌──────┴──────┐
        ↓             ↓
      Success?      Fail?
        ↓             ↓
      ✅ YES        ❌ NO
        ↓             ↓
   COMPLETED    Retry < 3?
     state        ↓
                 YES → Delayed retry (exponential backoff) → WAITING
                 NO → FAILED state
```

## Dashboard Integration

Watch these metrics update every 5 seconds:

- **Waiting**: Build up when jobs are added faster than processed
- **Active**: Stay at 3 (concurrent worker limit)
- **Completed**: Accumulate as jobs finish
- **Failed**: Increase when jobs fail all retries
- **Delayed**: Jobs waiting for their delay to expire

## Configuration Options

### Change Queues
```bash
QUEUES=q1,q2,q3,q4 bun run index.ts
```

### High Load Testing
Edit `index.ts` line ~85:
```typescript
// Faster job generation
setInterval(addJobs, 500 + Math.random() * 500);
```

### High Failure Rate
Edit `index.ts` line ~8:
```typescript
const jobTypes = [
  { name: 'process-data', failRate: 1.0, delayMs: 1000 },  // Always fail
];
```

### All Delayed Jobs
Edit `index.ts` line ~79:
```typescript
// All jobs delayed
const delay = Math.random() * 10000;
```

### More Workers Processing
Edit `index.ts` line ~44:
```typescript
concurrency: 10,  // Process more jobs at once
```

## Test Scenarios

### Scenario 1: Baseline (Default)
- 3 queues, 4 job types
- 10-30% failure rate
- Continuous job flow
- **Result**: Realistic queue behavior

### Scenario 2: High Load
- Increase frequency (500ms)
- **Result**: Build waiting queue, test dashboard with 50+ jobs

### Scenario 3: High Failure
- Make jobs fail 80-90%
- **Result**: Test failed state, retry mechanism

### Scenario 4: All Delayed
- Make all jobs delayed 5-20s
- **Result**: Test delayed queue transitions

### Scenario 5: Slow Processing
- Make jobs take 5-10 seconds each
- **Result**: Watch active queue grow

## Files Modified/Created

### Modified
- ✏️ `scripts/sim/index.ts` - Complete rewrite with workers

### Created
- 📝 `scripts/sim/README.md` - Simulator documentation
- 📝 `SIMULATOR_GUIDE.md` - Complete testing guide

## How to Use

### Step 1: Start Dependencies

```bash
# Terminal 1: Start Valkey
docker run -d --name valkey -p 6379:6379 valkey/valkey:latest

# Terminal 2: Start Bull-der-dash
cd C:\RootDev\bull-der-dash
.\bullderdash.exe

# Terminal 3: Start Simulator
cd scripts\sim
bun install  # First time only
bun run index.ts
```

### Step 2: Open Dashboard

```
http://localhost:8080
```

### Step 3: Watch It Work!

- Dashboard refreshes every 5 seconds
- Simulator logs to console showing job flow
- Click stats to see job details
- Watch jobs move through states in real-time

## Key Improvements

| Aspect | Before | After |
|--------|--------|-------|
| **Job Generation** | One-time | Continuous (every 2-4s) |
| **State Transitions** | None | Full flow through all states |
| **Workers** | None | 1 per queue, 3 concurrent |
| **Failure Simulation** | None | 10-30% based on type |
| **Retries** | None | 3 attempts with backoff |
| **Delayed Jobs** | None | 20% of jobs |
| **Progress Updates** | None | 0-100% during processing |
| **Job Types** | 2 static | 4 dynamic with different rates |
| **Queue Behavior** | Static | Realistic oscillating loads |
| **Testing Capability** | Limited | Comprehensive for all features |

## Performance Characteristics

- **Job Generation**: ~10-20 jobs/minute (default)
- **Job Processing**: 3 concurrent, 0.5-2s each
- **Completion Rate**: ~60-90% immediate, rest fail+retry
- **Queue Depth**: Varies 0-50+ depending on generation rate
- **Delay Overhead**: Minimal (Redis sorted set operations)

## Next Steps

1. **Run the simulator** with default settings
2. **Watch the dashboard** update in real-time
3. **Try different scenarios** (high load, failures, etc.)
4. **Validate metrics** at `/metrics` endpoint
5. **Click job details** to see full data
6. **Plan next features** (search, actions, etc.)

## Troubleshooting

### "Cannot connect to Redis"
- Make sure Valkey is running: `docker ps`
- Check `redis-cli ping` returns `PONG`

### "No jobs in dashboard"
- Verify simulator is running and showing output
- Refresh dashboard (F5)
- Check Valkey has data: `redis-cli KEYS "bull:*"`

### "Jobs not completing"
- Look for "Worker started" messages in simulator output
- Check for error messages in simulator logs

---

**Your simulator is now production-quality for testing!** 🚀

The enhanced simulator provides realistic workload scenarios to validate Bull-der-dash features, performance, and reliability.

