# 🚀 Complete End-to-End Testing Workflow

This document walks through the complete workflow of running Bull-der-dash with the enhanced simulator.

## The Full Picture

```
┌─────────────────────────────────────────────────────────────┐
│                   Your Testing Environment                  │
└─────────────────────────────────────────────────────────────┘
                              │
        ┌─────────────────────┼─────────────────────┐
        │                     │                     │
        ▼                     ▼                     ▼
   ┌─────────┐           ┌──────────────┐     ┌──────────────┐
   │  Valkey │           │Bull-der-dash │     │  Simulator   │
   │ (Redis) │◄──────────┤  Dashboard   │     │  (Workers)   │
   │6379     │           │ Port 8080    │     │              │
   └─────────┘           └──────────────┘     └──────────────┘
        │                     │                     │
        ├──────────────────────┴─────────────────────┤
        │                                            │
        │  BullMQ Data Structures                    │
        │  (queue states, job details, etc)         │
        │                                            │
        └────────────────────────────────────────────┘
```

## Step-by-Step Setup

### Phase 1: Environment Preparation

#### 1.1 Install Requirements
- ✅ Go 1.25+ (for Bull-der-dash)
- ✅ Bun or Node.js 18+ (for simulator)
- ✅ Docker (for Valkey)
- ✅ Redis CLI (for debugging)

#### 1.2 Verify Docker
```bash
docker --version
# Docker version 20.10+
```

#### 1.3 Build Bull-der-dash
```bash
cd C:\RootDev\bull-der-dash
go build -o bullderdash.exe .
# Creates: bullderdash.exe (20MB)
```

#### 1.4 Install Simulator Dependencies
```bash
cd scripts\sim
bun install
# Installs bullmq@5.68.0
```

### Phase 2: Starting Services (Terminal 1)

#### 2.1 Start Valkey Container

```bash
docker run -d \
  --name valkey \
  -p 6379:6379 \
  valkey/valkey:latest
```

Verify:
```bash
redis-cli ping
# Response: PONG
```

Check it's running:
```bash
docker ps | grep valkey
# Should show: valkey container running
```

**Status**: ✅ Valkey ready on `127.0.0.1:6379`

### Phase 3: Starting Dashboard (Terminal 2)

#### 3.1 Start Bull-der-dash

```bash
cd C:\RootDev\bull-der-dash
.\bullderdash.exe
```

Expected output:
```
🔧 Starting Bull-der-dash with config: Redis=127.0.0.1:6379, Port=8080, Prefix=bull
✅ Connected to Redis/Valkey
🚀 Bull-der-dash is running on http://localhost:8080
```

#### 3.2 Verify Dashboard Connection

```bash
# In another quick check terminal:
curl http://localhost:8080/health
# Should respond: OK
```

Check that Valkey is connected:
```bash
curl http://localhost:8080/ready
# Should respond: Ready
```

**Status**: ✅ Dashboard ready on `http://localhost:8080`

### Phase 4: Starting Simulator (Terminal 3)

#### 4.1 Start Job Simulator

```bash
cd C:\RootDev\bull-der-dash\scripts\sim
bun run index.ts
```

Expected output:
```
🎢 Starting Bull-der-dash Enhanced Job Simulator...
📋 Queues: orders, emails, billing
📊 Job types: process-data, send-email, webhook-call, database-sync

🔧 Worker started for queue: orders
🔧 Worker started for queue: emails
🔧 Worker started for queue: billing

📤 [orders] Added job a1b2c3d (process-data)
  ✅ [orders] Job completed
📤 [emails] Added job x9y8z7 (send-email) (delayed 3.2s)
```

**Status**: ✅ Simulator running and generating jobs

### Phase 5: Open Dashboard

#### 5.1 Launch Browser

Open: **http://localhost:8080**

#### 5.2 Initial Dashboard View

You should see:
- 🐂 Bull-der-dash Explorer header
- Queue list with statistics
- 3 queues: orders, emails, billing
- Numbers updating every 5 seconds

#### 5.3 Watch Real-Time Updates

The numbers change as:
- Waiting jobs get picked up by workers
- Active jobs complete or fail
- Completed jobs accumulate
- Failed jobs show after retries
- Delayed jobs transition to waiting

## Active Monitoring

### Dashboard Observations

#### After 10 seconds
```
Queue: orders
  Waiting:   5-10 jobs (queue building up)
  Active:    3 jobs (workers fully utilized)
  Completed: 2-5 jobs
  Failed:    0-1 jobs (some failures)
  Delayed:   1-2 jobs (awaiting delay)
```

#### After 30 seconds
```
Queue: orders
  Waiting:   2-8 jobs (queue oscillating)
  Active:    3 jobs (constant)
  Completed: 10-15 jobs (accumulating)
  Failed:    1-3 jobs
  Delayed:   1-3 jobs
```

#### After 2 minutes
```
Queue: orders
  Waiting:   Variable (5-20 depending on load)
  Active:    3 jobs (always)
  Completed: 40-60 jobs (accumulated)
  Failed:    5-10 jobs (natural failure rate)
  Delayed:   0-5 jobs
```

### Clicking Statistics

1. **Click on "10 Waiting"**
   - See list of jobs in waiting state
   - Most recent at top
   - Shows job name and creation time

2. **Click on "3 Active"**
   - See jobs being processed
   - Shows progress (0-100%)
   - Most will be at 50-75% progress

3. **Click on "50 Completed"**
   - See successfully completed jobs
   - Shows completion timestamp
   - Shows return value

4. **Click on "7 Failed"**
   - See failed jobs
   - Shows error message (Network timeout, Invalid data, etc.)
   - Shows attempt count

5. **Click on "2 Delayed"**
   - See jobs waiting for delay timer
   - Shows original queue

### Viewing Job Details

1. Select a job from any list
2. Click "View Details →"
3. See complete job information:

```json
{
  "id": "a1b2c3d",
  "name": "send-email",
  "data": {
    "value": 534,
    "user": "user-42"
  },
  "progress": 75,
  "state": "active",
  "attemptsMade": 1,
  "timestamp": 1708124400000,
  "processedOn": 1708124400500,
  "finishedOn": null
}
```

### Checking Metrics

Visit: **http://localhost:8080/metrics**

You'll see:
```
# Queue metrics
bullmq_queue_waiting_total{queue="orders"} 8
bullmq_queue_active_total{queue="orders"} 3
bullmq_queue_completed_total{queue="orders"} 52
bullmq_queue_failed_total{queue="orders"} 7
bullmq_queue_delayed_total{queue="orders"} 2

# HTTP metrics
http_request_duration_seconds_bucket{method="GET",path="/queues",status="200",le="0.005"} 142

# Redis metrics
redis_operation_duration_seconds_bucket{operation="get_queue_stats",le="0.01"} 145
```

## Test Scenarios

### Scenario 1: Baseline Operation (Default)

**Duration**: 2 minutes  
**Observation**:
- Queue depths oscillate naturally
- Active always at 3 (concurrent limit)
- Completed grows steadily
- Some failures occur
- Delayed jobs transition properly

**Validation**:
- ✅ Dashboard updates smoothly
- ✅ All states represented
- ✅ Metrics are accurate
- ✅ No errors in logs

### Scenario 2: High Load Test

**Setup**:
```bash
# Edit scripts/sim/index.ts
setInterval(addJobs, 500 + Math.random() * 500);  # 4x faster
```

**Duration**: 2 minutes  
**Observation**:
- Waiting queue builds to 20-40+ jobs
- Active stays at 3
- Completed grows slower (backlog)
- Dashboard remains responsive

**Validation**:
- ✅ Dashboard handles large queues
- ✅ No lag in updates
- ✅ Metrics remain accurate
- ✅ No memory leaks

### Scenario 3: Failure Rate Test

**Setup**:
```bash
# Edit scripts/sim/index.ts
const jobTypes = [
  { name: 'process-data', failRate: 0.9, delayMs: 1000 },  # 90% fail!
```

**Duration**: 2 minutes  
**Observation**:
- Failed count increases rapidly
- Completed grows slowly
- Jobs retry with backoff
- Some jobs remain failed

**Validation**:
- ✅ Retry mechanism works
- ✅ Failed state tracking correct
- ✅ Error messages appear
- ✅ Dashboard shows failures

### Scenario 4: Delayed Job Test

**Setup**:
```bash
# Edit scripts/sim/index.ts
const delay = Math.random() * 10000;  # ALL jobs delayed
```

**Duration**: 3 minutes  
**Observation**:
- Delayed count spikes
- Waiting stays low initially
- Active stays at 0 until delays expire
- Over time, jobs transition to waiting
- Then process normally

**Validation**:
- ✅ Delayed state implemented
- ✅ Transitions to waiting happen
- ✅ Job scheduling works
- ✅ No premature processing

## Debugging & Troubleshooting

### Dashboard Not Updating

**Check**:
```bash
# Is Bull-der-dash responding?
curl http://localhost:8080/queues

# Is Valkey connected?
redis-cli KEYS "bull:*"  # Should show many keys
```

**Fix**:
- Refresh browser (F5)
- Check both terminals still running
- Restart Bull-der-dash if needed

### No Jobs Appearing

**Check**:
```bash
# Is simulator running?
# Should see "📤 Added job" messages

# Does Valkey have data?
redis-cli KEYS "bull:*" | wc -l  # Should be > 0

# Are workers running?
# Should see "Worker started" messages
```

**Fix**:
- Restart simulator
- Verify Valkey connection
- Check for error messages

### Jobs Not Moving Through States

**Check**:
```bash
# Look at simulator console output
# Should see ✅ completed and ❌ failed messages

# Check individual job:
redis-cli HGETALL "bull:orders:{jobId}"
```

**Fix**:
- Verify workers started
- Check job processing time isn't too long
- Look for error messages in output

### Dashboard Slow/Laggy

**Check**:
- Reduce load: Increase interval in simulator
- Check dashboard resource usage
- Verify network connectivity

**Fix**:
- Tune job frequency
- Monitor system resources
- Reduce queue size if testing

## Cleanup

### Stop Everything Gracefully

```bash
# Terminal 3: Stop simulator
Ctrl+C

# Terminal 2: Stop Bull-der-dash
Ctrl+C

# Terminal 1: Stop Valkey
docker stop valkey
docker rm valkey

# Optional: Clean up images
docker image prune
```

### Clean Redis Data (Full Reset)

```bash
# WARNING: Deletes all Bull-der-dash data!
redis-cli FLUSHDB

# Or specific queues:
redis-cli DEL bull:orders:* bull:emails:* bull:billing:*
```

## Performance Baseline

For reference, these are expected metrics:

| Metric | Value | Range |
|--------|-------|-------|
| Dashboard response time | <10ms | 5-20ms |
| Metrics endpoint | <5ms | 1-10ms |
| Queue discovery | <20ms | 10-50ms |
| Job detail fetch | <15ms | 5-30ms |
| Dashboard memory | 20-30MB | 15-50MB |
| Simulator memory | 30-50MB | 20-100MB |
| Valkey memory | 10-20MB | 5-100MB* |

*Depends on data retained

## Success Criteria

✅ All of these should work:

- [ ] Dashboard loads and shows queues
- [ ] Queue statistics update every 5 seconds
- [ ] Clicking stats shows job list
- [ ] Clicking jobs shows details
- [ ] Progress updates during processing
- [ ] Failed jobs show error messages
- [ ] Delayed jobs eventually process
- [ ] Metrics endpoint works
- [ ] Health checks respond
- [ ] Dashboard remains responsive under load

## Next Steps

1. ✅ Get baseline operation working
2. 📊 Test each scenario
3. 📈 Monitor metrics and performance
4. 🎨 Try customizing the simulator
5. 🧪 Plan for search feature
6. 🎯 Plan for action buttons

---

**Congratulations!** You now have a complete testing environment for Bull-der-dash! 🎉

The combination of Bull-der-dash, the enhanced simulator, and Valkey gives you a production-like testing environment to develop and validate your queue monitoring dashboard.

Happy testing! 🚀

