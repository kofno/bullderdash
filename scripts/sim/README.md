# Bull-der-dash Job Simulator

A realistic BullMQ job simulator for exercising queue states, retry behavior, and parent-child flows.

## Features
- Per-queue arrival rates (Poisson-like)
- Burst traffic and priorities
- Retries with exponential backoff
- Delayed jobs
- Parent-child flows (waiting-children)
- Occasional queue pauses (paused state)
- Multiple job types with realistic timings and failure rates

## Quick Start

### Prerequisites
- Bun
- Valkey/Redis running on `127.0.0.1:6379`
- Bull-der-dash running on `http://localhost:8080`

### Run
```bash
cd scripts/sim
bun install
bun run index.ts
```

## Configuration

### Environment Variables
```bash
# Specify which queues to create (default: orders,emails,billing)
QUEUES=orders,emails,billing,payments bun run index.ts

# Retries per job before a job is permanently FAILED (default: 3)
MAX_ATTEMPTS=2 bun run index.ts

# Scale every job type's per-attempt failure rate (default: 1)
# e.g. 2 doubles failure rates to surface more FAILED jobs quickly
FAIL_RATE_MULTIPLIER=2 bun run index.ts
```

## Job Types

| Job Type | Fail Rate | Avg Time | Notes |
|----------|-----------|----------|-------|
| `process-data` | 5% | 3s | Data processing |
| `send-email` | 8% | 1.5s | Email delivery |
| `webhook-call` | 12% | 3.5s | External APIs |
| `database-sync` | 3% | 2.8s | Database operations |
| `report-generate` | 6% | 6s | Longer-running |
| `order-finalize` | 2% | 2.5s | Parent job for flows |
| `flaky-task` | 70% | 1.2s | Deliberately flaky — reliably exhausts retries into `FAILED` |

> `flaky-task` exists so the `FAILED` category (and multi-attempt
> retried-then-completed jobs) populate quickly. With the default
> `MAX_ATTEMPTS=3`, roughly `0.7^3 ≈ 34%` of flaky jobs fail permanently while
> the rest complete after 1–2 retries, exercising the attempt counter.

## Flow Simulation

The simulator periodically creates a parent job with children:
- Parent: `order-finalize` (typically in `orders`)
- Children: `process-data`, `database-sync`, `send-email`

This reliably produces the `waiting-children` state.

## Tuning

Profiles and job mix live in `scripts/sim/index.ts`:
- `queueProfiles` controls per-queue concurrency and arrival rate
- `jobMix` controls weighted job selection
- `burstChance` and `burstMultiplier` control spikes

If you want higher load, reduce `meanIntervalMs` or increase `concurrency`.
To see more `FAILED` jobs without editing code, raise `FAIL_RATE_MULTIPLIER`
or lower `MAX_ATTEMPTS`.

## Bounded history reliability workload

`reliability.ts` is a separate, repeatable high-throughput mode. It leaves the
interactive simulator unchanged and drives **real BullMQ workers**, not direct
SQLite inserts. Use it with a local dashboard binary configured with workload
metrics and history enabled to exercise Redis events, collector lookups, async
writes, FTS search and retention together.

It only accepts a loopback Redis URL, requires a fresh queue, retains all job
hashes and terminal sets for reconciliation, and writes a JSON manifest of
completed/failed IDs and per-phase counts. Every twentieth job fails permanently
on its single attempt; the rest complete after a 2ms simulated task. Payloads
include ~1KiB of deterministic per-job sensor tokens and `reliabilityneedle`
for full-payload FTS queries. Concurrency is 128. There are no retries, flows,
auto-removal, progress events or random pauses in this mode.

Start a disposable, loopback-bound local instance (not an existing Redis):

```powershell
docker run --detach --name bdd-history-sim --publish 127.0.0.1:16389:6379 valkey/valkey:8-alpine
docker exec bdd-history-sim valkey-cli ping
task sim-install
```

Run the dashboard in another terminal:

```powershell
$env:REDIS_ADDR = '127.0.0.1:16389'
$env:SERVER_PORT = '18089'
$env:WORKLOAD_METRICS_ENABLED = 'true'
$env:WORKLOAD_METRICS_POLL_SECONDS = '1'
$env:WORKLOAD_METRICS_START_ID = '$'
$env:STORE_ENABLED = 'true'
$env:STORE_DB_PATH = Join-Path $env:TEMP 'bdd-history-sim.db'
$env:STORE_WRITE_BUFFER = '16384'
$env:STORE_BATCH_SIZE = '512'
$env:STORE_FLUSH_MILLIS = '500'
$env:STORE_COMPLETED_TTL_HOURS = '1'
$env:STORE_FAILED_TTL_HOURS = '336'
$env:STORE_SWEEP_SECONDS = '10'
$env:STORE_SWEEP_BATCH = '2000'
$env:STORE_SWEEP_PAUSE_MILLIS = '25'
task build
.\bullderdash.exe
```

From the repository root, start the bounded simulator:

```powershell
$env:REDIS_URL = 'redis://127.0.0.1:16389'
$env:SIM_QUEUE = 'history-reliability'
$env:SIM_OUTPUT = Join-Path $env:TEMP 'bdd-history-sim-result.json'
Push-Location scripts\sim
bun run reliability.ts
Pop-Location
```

| Setting | Default | Meaning |
|---------|---------|---------|
| `SIM_QUEUE` | `history-reliability` | Fresh BullMQ queue; an existing `id` key is rejected |
| `SIM_OUTPUT` | `reliability-result.json` | Manifest path; use an absolute path outside the checkout |
| `SIM_START_DELAY_SECONDS` | `5` | Allow queue discovery/XREAD to begin before the first job when starting at `$` |
| `SIM_STEADY_RATE` | `78` | Jobs/s during steady/recovery, yielding about 74.1 completions/s and 3.9 failures/s |
| `SIM_STEADY_SECONDS` | `65` | Initial steady phase |
| `SIM_BURST_RATE` | `2000` | Burst jobs/s (the requested rate, not a guaranteed achieved throughput) |
| `SIM_BURST_SECONDS` | `15` | Burst phase |
| `SIM_RECOVERY_SECONDS` | `35` | Return to steady rate |
| `SIM_DRAIN_SECONDS` | `120` | Maximum worker-drain wait after producing |

The collector's initial sweep starts after 30s, then repeats on the configured
interval. The example's one-hour completed TTL **does not expire new jobs during
this two-minute run**. To exercise DELETE/FTS contention immediately, give each
build a copy of the same pre-seeded history database with completed records
older than one hour. Keep seed rows in a different queue so they are not confused
with live BullMQ counts. New rows must remain younger than the TTL when comparing
terminal IDs; a retention deletion is not a persistence loss.

Scrape `/metrics` during the run; sample queue depth and event lag while load
is active, not only after draining. Reconcile the manifest's terminal ID sets
against `job_history` for `SIM_QUEUE`, including states. Check
`/console`, `/v1/search?q=reliabilityneedle&limit=1000` and an individual
`/v1/jobs/sim-100` detail. The event stream maxLen is raised to 1,000,000
(BullMQ's default is much smaller); inspect stream length/first entry to verify
events were not trimmed before attributing missing records to the store.
This mode measures ingestion under a controlled workload, not the full
interactive simulator's retry/flow behaviors.

After stopping the dashboard, remove only the disposable container you created:

```powershell
docker stop bdd-history-sim
docker rm bdd-history-sim
```

### Measured baseline comparison (2026-10-25)

Both the `v0.2.1` tag (`e62d62f`) and phase 1 (`dff98ae`) were built with
Go 1.26.0 and run sequentially on Windows/amd64 (Ryzen 7 PRO 7840U), using
Bun 1.2.14, BullMQ 5.68.0 and a disposable Docker Valkey 8.1.10 instance.
They used the settings above, plus a collector batch of 100 and 1s XREAD block,
with identical copies of a 150,000-row baseline-schema database: 90k fresh
completed, 10k fresh failed, and 50k completed more than two hours old.
Seed rows used the same 96-sensor payload shape, with FTS triggers enabled.
The fixture was ~315.5MiB; phase 1 built its indexes at startup.

Each run offered 5,070 jobs over 65s, 30,000 over 15s, then 2,730 over 35s.
Observed steady completions were ~74.1/s; burst completions were ~1,887/s
(with ~99.5 failures/s and 143 jobs still waiting/active at the burst boundary).
New terminal rows were not eligible for retention.

| Measurement | v0.2.1 | Phase 1 |
|-------------|--------|---------|
| BullMQ completed / failed | 35,910 / 1,890 | 35,910 / 1,890 |
| SQLite completed / failed, exact live ID/state reconciliation | 35,910 / 1,890; no missing IDs | 35,910 / 1,890; no missing IDs |
| Full-channel drops / failed write batches | 0 / 0 | 0 / 0 |
| Explicit error/shutdown losses / SQLite retries | Not instrumented | 0 losses; no retry series instantiated (no retries observed) |
| Sampled peak channel depth / capacity | Not instrumented | 565 / 16,384 |
| Expired seed rows deleted / retained seed rows left | 50,000 / 100,000 | 50,000 / 100,000 |
| Sweeps | Repeated at 10s after the initial sweep; no duration counter | 9 completed; 43 DELETE attempts including empty batches |
| Peak sampled event lag | 14.385s | 14.455s |
| Final event lag | 6.95ms | 6.76ms |
| Mean ingest batch duration | 99.3ms (255 batches) | 91.8ms (257 batches) |
| Mean admission wait, ingest / sweep | Not instrumented | 10.1ms / 33.7ms per attempt |
| Mean DELETE attempt / first whole sweep duration | Not instrumented | 170.9ms / 8.600s |
| All sweep durations combined | Not instrumented | 9.432s across 9 sweeps, including admission/pacing |
| Payload FTS search / single-job trace search / console HTML | Passed | Passed |
| Mean / maximum sampled payload-search HTTP time | 61.8ms / 145.3ms | 60.0ms / 149.0ms |
| FTS external-content integrity after graceful shutdown | Passed | Passed |

The console's search API returned 1,000 matching live jobs at its limit, and
`trace_id=simtrace-100` returned the expected failed job. Both collectors reported
the same 35,910/1,890 terminal events as BullMQ. Event stream lengths were
154,129/154,134, below the 1,000,000 cap; `entries-added` equalled length and
the first entry was `sim-0`, so neither stream trimmed events. Stream lengths
differ because timing-dependent `drained` events are also recorded.
The final real SQLite schemas reproduced the old state-only failed seek and
completed covering-index scan, versus per-tier expression range seeks in phase 1.

**This run did not reproduce production drops in either build.** It establishes
end-to-end correctness under this workload, not a demonstrated drop-rate or
throughput improvement. The main expired backlog was deleted during the initial
steady phase; later sweeps, including those during the burst, checked for newly
eligible rows but found none. The 0.5s metric scrape interval can miss short
queue/lag peaks, and one sequential run per build is not a statistical benchmark.
Local warm disk, ~150k seed rows, a ~two-minute workload and deterministic ~1KiB
payloads do not model managed-csi latency, millions of retained rows, long-running
WAL/checkpoint behavior, retry/flow fan-out, event trimming, or process crashes.
Measured startup-to-HTTP-ready times (0.53s / 2.23s) include local cache and index
build effects and must not be used to promise a production migration duration.

## Troubleshooting

- **Cannot connect to Valkey**: ensure Redis is running on `127.0.0.1:6379`.
- **No jobs in dashboard**: refresh and confirm simulator is running.
- **Not seeing waiting-children**: wait for the flow interval (45–90s).
