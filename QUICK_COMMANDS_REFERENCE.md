# 🎯 Quick Command Reference

## Running Everything

### Terminal 1: Start the Web Application
```bash
./bullderdash.exe
# Opens http://localhost:8080
```

### Terminal 2: Start the Simulator
```bash
cd scripts/sim
npm run start
```

### Terminal 3: Monitor Queue with Redis CLI
```bash
./redis-cli.exe
```

---

## Redis CLI Commands

### View Queue Statistics
```redis
QUEUE-STATS orders
QUEUE-STATS emails
QUEUE-STATS billing
```

**Output shows:**
- 🕐 Waiting jobs
- 🚀 Active jobs
- ✅ Completed jobs
- ❌ Failed jobs
- ⏰ Delayed jobs
- 🔒 Stalled jobs
- 👻 Orphaned jobs
- 📊 Total jobs

### List All Keys
```redis
KEYS bull:orders:*
KEYS bull:emails:*
KEYS bull:billing:*
```

### View Specific Queue States
```redis
# Waiting jobs
LRANGE bull:orders:wait 0 -1

# Active jobs
LRANGE bull:orders:active 0 -1

# Delayed jobs
LRANGE bull:orders:delayed 0 -1

# Failed jobs
SMEMBERS bull:orders:failed

# Completed jobs
SMEMBERS bull:orders:completed

# Stalled jobs
SMEMBERS bull:orders:stalled
```

### View Job Details
```redis
HGETALL bull:orders:1
HGETALL bull:orders:2
```

### Count Jobs by State
```redis
LLEN bull:orders:wait
LLEN bull:orders:active
SCARD bull:orders:failed
SCARD bull:orders:completed
ZCARD bull:orders:delayed
ZCARD bull:orders:stalled
```

---

## Web UI URLs

### Dashboard (All Queues)
```
http://localhost:8080
```
Shows all queues with:
- Queue name
- Waiting count
- Active count
- Completed count
- Failed count
- Delayed count
- Stalled count
- Orphaned count
- Total count

### Queue Details
```
http://localhost:8080/queue/orders
http://localhost:8080/queue/emails
http://localhost:8080/queue/billing
```
Shows:
- Summary table with all states
- Job lists for each state
- Job details

### Prometheus Metrics
```
http://localhost:8080/metrics
```
Shows all Prometheus metrics including:
- bullmq_queue_waiting_total
- bullmq_queue_active_total
- bullmq_queue_completed_total
- bullmq_queue_failed_total
- bullmq_queue_delayed_total
- bullmq_queue_stalled_total
- bullmq_queue_orphaned_total

### Health Check
```
http://localhost:8080/health
```

### Ready Check
```
http://localhost:8080/ready
```

---

## Simulator Configuration

### Environment Variables
```bash
# Customize queues
export QUEUES=orders,emails,billing,payments

# Then start simulator
npm run start
```

### Job Types (Built-in)
- `process-data` - 3s, 15% failure
- `send-email` - 2s, 25% failure
- `webhook-call` - 4s, 35% failure
- `database-sync` - 2.5s, 10% failure
- `report-generate` - 5s, 20% failure

### Timing
- **Job processing:** 2-5 seconds
- **New jobs added:** Every 8-12 seconds
- **Retry attempts:** Up to 3 per job
- **Delayed jobs:** 60% immediate, 30% 5-15s, 10% 30-60s

---

## Typical Workflow

### Step 1: Start Everything
```bash
# Terminal 1
./bullderdash.exe

# Terminal 2 (wait 2 seconds)
cd scripts/sim && npm run start

# Terminal 3
./redis-cli.exe
```

### Step 2: Monitor Queue
```bash
# In redis-cli
QUEUE-STATS orders

# Watch values change as simulator runs
```

### Step 3: View Dashboard
```
Open: http://localhost:8080
Watch dashboard refresh every 5 seconds
```

### Step 4: View Queue Details
```
Open: http://localhost:8080/queue/orders
See all job states and job lists
```

---

## Stopping Everything

### Stop Simulator
```bash
# In Terminal 2, press:
Ctrl+C
```

### Stop Web App
```bash
# In Terminal 1, press:
Ctrl+C
```

### Exit Redis CLI
```bash
# In Terminal 3, type:
> EXIT
```

---

## Troubleshooting Commands

### Check Redis Connection
```redis
PING
# Should return: PONG
```

### Count Total Keys
```redis
DBSIZE
```

### Clear All Keys (CAREFUL!)
```redis
FLUSHDB
# Clears everything
```

### Check Queue Status
```redis
QUEUE-STATS orders
QUEUE-STATS emails
QUEUE-STATS billing
```

### View Recent Logs (in simulator terminal)
```
Ctrl+C to stop and see history
```

---

## Expected Queue Stats

### Fresh Start (First 10 seconds)
```
Waiting:   2-5
Active:    1
Completed: 0
Failed:    0
Delayed:   1-2
Total:     4-8
```

### Running Normally (30+ seconds)
```
Waiting:   8-15
Active:    1
Completed: 5-10
Failed:    2-3
Delayed:   3-8
Stalled:   0
Orphaned:  0
Total:     20-40
```

### High Load (1+ minute)
```
Waiting:   12-20
Active:    1
Completed: 15-25
Failed:    5-8
Delayed:   5-10
Stalled:   0-1
Orphaned:  0
Total:     40-65
```

---

## Performance Metrics

### Dashboard Refresh
- Auto-refreshes every 5 seconds
- Fast load (< 500ms)
- Handles 40+ jobs easily

### Simulator Performance
- Memory: ~50-100MB
- CPU: Low (background)
- Job creation: Non-blocking

### Redis Operations
- Query time: <100ms typical
- Scan operations: Paginated (100 keys)
- Orphaned detection: Efficient

---

## Common Use Cases

### Demonstrate Queue System
```bash
1. Start app and simulator
2. Open http://localhost:8080
3. Show queue building up
4. Show jobs in different states
5. Run QUEUE-STATS in redis-cli
```

### Test Failure Handling
```bash
1. Watch FAILED count increase (10-35% failure rate)
2. See jobs retry
3. Track completion after retries
```

### Test Delayed Jobs
```bash
1. Run: LRANGE bull:orders:delayed 0 -1
2. Watch jobs move from delayed to waiting
3. See them process after delay
```

### Monitor Performance
```bash
1. Open: http://localhost:8080/metrics
2. Watch Prometheus metrics update
3. Track job throughput
```

---

## Useful Patterns

### Check Queue Depth Over Time
```redis
QUEUE-STATS orders
# Run multiple times, watch total grow then stabilize
```

### Watch Failed Jobs
```redis
SMEMBERS bull:orders:failed
# See which jobs are retrying
```

### Monitor Progress
```redis
QUEUE-STATS orders
QUEUE-STATS emails
QUEUE-STATS billing
# Check all queues simultaneously
```

---

## Tips & Tricks

### ⏸️ Pause Simulator Without Stopping
- Ctrl+Z (suspend) in simulator terminal
- Ctrl+C then restart to resume

### 📊 Watch Live Statistics
```bash
# Keep rerunning QUEUE-STATS
QUEUE-STATS orders
# (repeat every few seconds manually)
```

### 🔍 Debug Individual Jobs
```redis
HGETALL bull:orders:123
# See all job details
```

### 📈 Track Job Completion Rate
```redis
QUEUE-STATS orders
# Completed count should increase every 2-5s
```

---

## You're Ready! 🚀

Everything is set up and working. Just:
1. Run the three terminals
2. Watch your queue system work
3. Explore the dashboard
4. Use redis-cli to inspect details

**Happy monitoring!**

