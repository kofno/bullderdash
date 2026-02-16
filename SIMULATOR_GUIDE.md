# Testing Bull-der-dash with the Enhanced Simulator

This guide walks you through using the improved job simulator to test your Bull-der-dash dashboard.

## Overview

The enhanced simulator:
- ✅ Creates workers that **process jobs through different states**
- ✅ Generates jobs continuously for realistic workload
- ✅ Simulates failures and retries
- ✅ Moves jobs through **all BullMQ states**:
  - 🕐 Waiting (queued, waiting for processing)
  - 🚀 Active (currently being processed with progress)
  - ✅ Completed (successfully finished)
  - ❌ Failed (errors, end of retry chain)
  - ⏰ Delayed (scheduled for future)

## Complete Setup Guide

### Step 1: Start Valkey (Redis)

```bash
docker run -d --name valkey -p 6379:6379 valkey/valkey:latest
```

Verify:
```bash
redis-cli ping
# Should return: PONG
```

### Step 2: Start Bull-der-dash Dashboard

```bash
cd C:\RootDev\bull-der-dash

# Option A: Use pre-built binary
.\bullderdash.exe

# Option B: Build and run
go build -o bullderdash.exe .
.\bullderdash.exe
```

You should see:
```
🔧 Starting Bull-der-dash with config: Redis=127.0.0.1:6379, Port=8080, Prefix=bull
✅ Connected to Redis/Valkey
🚀 Bull-der-dash is running on http://localhost:8080
```

### Step 3: Start the Simulator

```bash
cd C:\RootDev\bull-der-dash\scripts\sim

# Install dependencies (first time only)
bun install

# Run simulator
bun run index.ts
```

You should see:
```
🎢 Starting Bull-der-dash Enhanced Job Simulator...
📋 Queues: orders, emails, billing
📊 Job types: process-data, send-email, webhook-call, database-sync

🔧 Worker started for queue: orders
🔧 Worker started for queue: emails
🔧 Worker started for queue: billing

📤 [orders] Added job a1b2c3d (process-data)
📤 [emails] Added job x9y8z7f (send-email) (delayed 3.2s)
  ✅ [orders] Job 1 (process-data) completed
```

### Step 4: Open Dashboard

Open your browser to: **http://localhost:8080**

You should see:
- 🐂 **Bull-der-dash Explorer** header
- Queue statistics updating every 5 seconds
- Live counts for Waiting, Active, Completed, Failed, Delayed

## What to Watch

### Dashboard Live Updates

As the simulator runs, watch the numbers change:

```
Queue: orders
  Waiting:   15 → 12 → 8 → 5    (jobs being picked up by workers)
  Active:    3 → 3 → 3 → 2      (concurrent processing)
  Completed: 42 → 45 → 48       (accumulating)
  Failed:    2 → 2 → 3 → 3      (after retries)
  Delayed:   4 → 3 → 2 → 1      (moving to waiting when time comes)
```

### Click to Explore

1. **Click on "15 Waiting"** - See queued jobs waiting for processing
2. **Click on "3 Active"** - See jobs currently being processed with progress
3. **Click on "48 Completed"** - See successfully processed jobs
4. **Click on "3 Failed"** - See jobs that failed after retries
5. **Click on "2 Delayed"** - See jobs scheduled for future

### View Job Details

1. Select a job from the list
2. Click "View Details →"
3. See complete job data including:
   - Job ID and Name
   - Input data (value, user)
   - Current state
   - Progress (0-100% for active jobs)
   - Error messages (for failed jobs)
   - Attempt count

## Test Scenarios

### Scenario 1: Basic Flow

Watch the default simulator:
- Jobs added every 2-4 seconds
- Most jobs complete successfully
- Some fail and are retried
- Some are delayed 3-10 seconds

**Expected behavior**: Queue depths oscillate, jobs flow through states

### Scenario 2: High Load

Increase job generation frequency:

Edit `scripts/sim/index.ts` and change:
```typescript
// FROM:
setInterval(addJobs, 2000 + Math.random() * 2000);

// TO:
setInterval(addJobs, 500 + Math.random() * 500);  // 10x faster!
```

Then restart simulator.

**What to watch**:
- Waiting queue builds up significantly
- Active stays at 3 (concurrent limit)
- Dashboard updates every 5 seconds
- System remains responsive

### Scenario 3: High Failure Rate

Make jobs more likely to fail:

Edit `scripts/sim/index.ts`:
```typescript
const jobTypes = [
  { name: 'process-data', failRate: 0.8, delayMs: 1000 },      // 80% fail!
  { name: 'send-email', failRate: 0.9, delayMs: 500 },          // 90% fail!
  // ...
];
```

**What to watch**:
- Failed count increases rapidly
- Retry system working (jobs retry 3 times)
- Error messages in job details

### Scenario 4: All Delayed

Test delayed job handling:

Edit `scripts/sim/index.ts`:
```typescript
// FROM:
const delay = Math.random() < 0.2 ? Math.random() * 10000 : 0;

// TO:
const delay = Math.random() * 10000;  // ALL jobs delayed!
```

**What to watch**:
- Delayed count spikes
- Jobs don't enter active until delay expires
- Watch delayed jobs transition to waiting

### Scenario 5: Slow Processing

Test with longer processing times:

Edit `scripts/sim/index.ts`:
```typescript
const jobTypes = [
  { name: 'process-data', failRate: 0.1, delayMs: 10000 },  // 10 seconds!
  // ...
];
```

**What to watch**:
- Active jobs take longer to complete
- Progress updates every 100ms
- Waiting queue builds up

## Verifying Everything Works

### Check 1: Queue Discovery

Dashboard should show your queues immediately:
- ✅ orders
- ✅ emails
- ✅ billing

### Check 2: Job Flow

Watch jobs move through states:
- ✅ Jobs appear in "Waiting" first
- ✅ Move to "Active" when processing
- ✅ Go to "Completed" on success
- ✅ Go to "Failed" after retries
- ✅ "Delayed" count changes over time

### Check 3: Real-Time Updates

Dashboard updates show live changes:
- ✅ Numbers change every 5 seconds
- ✅ No page reload needed
- ✅ Smooth transitions

### Check 4: Job Details

Click through to see job information:
- ✅ Job ID, name, data
- ✅ Current state
- ✅ Progress for active jobs
- ✅ Error messages for failed jobs

### Check 5: Metrics

Visit `http://localhost:8080/metrics` to see Prometheus data:
- ✅ bullmq_queue_waiting_total
- ✅ bullmq_queue_active_total
- ✅ bullmq_queue_failed_total
- ✅ bullmq_queue_completed_total
- ✅ bullmq_queue_delayed_total

## Debugging

### Simulator shows "Cannot connect to Redis"
- Verify Docker container is running: `docker ps | grep valkey`
- Try `redis-cli ping` to test connection
- Check `127.0.0.1:6379` is accessible

### No jobs appearing in dashboard
- Refresh the page (F5)
- Check simulator is running and showing "📤 Added job" messages
- Verify database connection works: `redis-cli KEYS "bull:*"`

### Jobs not moving through states
- Check simulator console for "Worker started" messages
- Verify no errors in simulator output
- Look for "✅ completed" or "❌ failed" messages

### Dashboard not updating
- Check it's polling correctly (HTMX should show network activity)
- Refresh the page
- Check browser console for errors (F12)

## Cleanup

To stop everything:

```bash
# Stop simulator (Terminal 3)
Ctrl+C

# Stop Bull-der-dash (Terminal 2)
Ctrl+C

# Stop Valkey container (Terminal 1)
docker stop valkey
docker rm valkey
```

## Files Involved

- `C:\RootDev\bull-der-dash\bullderdash.exe` - Dashboard binary
- `C:\RootDev\bull-der-dash\scripts\sim\index.ts` - Simulator code
- `http://localhost:8080` - Dashboard UI
- `http://localhost:8080/metrics` - Prometheus metrics
- `redis-cli` - Redis command line (for debugging)

## Next Steps

Once you're comfortable with the simulator:

1. **Test Search** (when implemented)
   - Add jobs with specific data
   - Test finding them by keywords

2. **Test Actions** (when implemented)
   - Retry failed jobs
   - Remove specific jobs
   - Pause/resume queues

3. **Test Alerts** (when implemented)
   - Set thresholds on queue depths
   - Monitor failed job spikes

4. **Performance Testing**
   - Increase simulator load
   - Watch dashboard responsiveness
   - Check Prometheus metrics scraping

---

**Enjoy testing!** 🚀

The simulator provides a realistic test bed for developing and validating Bull-der-dash features.

