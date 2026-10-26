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
| `SIM_STREAM_MAXLEN` | `1000000` | Producer event retention cap (BullMQ trims approximately); use a deliberately small value only for gap testing |

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

### Production-scale contention runner

`compare.py` automates paired Windows runs using two pre-built binaries. It uses
only loopback addresses and verifies that the explicitly named disposable Valkey
container is published at `127.0.0.1:16389`. **It clears that container's Redis
database between builds.** Do not supply a shared container. Python with SQLite
FTS5 support, Docker, Bun and the simulator dependencies are required.

The default fixture contains 3,000,000 history rows: 2,000,000 retained (90%
completed / 10% failed) and 1,000,000 completed more than 48 hours old, with the
same ~1KiB payload shape and FTS triggers enabled. It is built once using the
baseline schema and copied per build. Outputs include complete terminal-ID
manifests, ~0.5s metric snapshots, query timings, application logs, competing
writer intervals, final summaries and SQLite databases. Allow substantial local
disk space for the fixture, two copies, indexes, WAL files and build artifacts.

Build the exact baseline and phase 1 binaries into an artifact directory, not
over one another; then run from the repository root:

```powershell
docker run --detach --name bdd-history-stress --publish 127.0.0.1:16389:6379 valkey/valkey:8-alpine
python scripts\sim\compare.py `
  --output C:\Temp\bdd-history-stress `
  --baseline C:\Temp\bdd-v021.exe `
  --phase1 C:\Temp\bdd-phase1.exe `
  --container bdd-history-stress
```

The runner uses buffer=16,384, batch=512, flush=500ms, completedTTL=1h,
failedTTL=336h, sweepBatch=2,000, sweepPause=25ms and a 10s sweep interval with
a 300s per-sweep deadline. The initial sweep starts after 30s. A 26s steady
phase follows a 5s discovery lead-in, so the 60s burst at 2,000 offered jobs/s
starts around the first sweep. A 60s recovery follows at 78 jobs/s. A separate
SQLite connection tries to acquire a write transaction from t=29s through t=119s,
holds acquired transactions for ~400ms and pauses for 100ms between attempts.
Its acquisition/hold timestamps distinguish actual lock exposure from scheduled
contention; failed acquisitions are counted, not assumed to have held the lock.
The write affects a dedicated diagnostic table in the same database file,
not job-history rows or FTS contents.

The runner samples through collector catch-up and retention drain, up to a
420s workload deadline. It reports missing jobs even when losses occurred,
rather than asserting zero drops as a prerequisite for a usable comparison.
Inspect `seed_counts_after` and the final sweep counter before claiming the
entire expired backlog drained. Startup timing is process-launch to HTTP-ready,
including migration/index creation, not an isolated index-only benchmark.

For a separately labeled smaller-buffer experiment, rerun with `--buffer 2048`
and the same output directory. It reuses the verified seed and produces separate
`*-buffer2048.*` artifacts. Completed summary files are reused rather than
overwritten; inspect or move an incomplete run deliberately before retrying.
New runs reject a seed whose retained timestamp has aged past the configured
completed TTL; build a fresh fixture rather than accidentally expiring the
retained tier in a later variant.
`--rows` and `--expired` permit a smaller fixture when resource constraints
require it; always report the actual counts and sizes. After all runs:

```powershell
docker stop bdd-history-stress
docker rm bdd-history-stress
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

### Measured 3M-row contention stress (2026-10-25)

The runner above was then used against the same binaries (`v0.2.1` / `dff98ae`)
on the same host, with **3,000,000 seed rows and 1,000,000 expired completed
rows**. The other 2,000,000 rows were fresh (1.8M completed / 200k failed).
The baseline-schema fixture, including payload FTS and indexes, occupied
6,575,271,936 bytes (6.12GiB). Building it through real FTS triggers, checking
external-content integrity and checkpointing took **978.2s**; inserting the last
row finished at 940.9s. One 100k-row insertion interval was much slower than
others, so the build was not a uniform throughput benchmark.

Both runs used buffer=16,384, batch=512, sweepBatch=2,000, sweepPause=25ms,
flush=500ms, completedTTL=1h, failedTTL=336h, sweep interval=10s and a 300s
per-sweep deadline. Each offered **126,708 jobs**: 2,028 steady, 120,000 burst,
4,680 recovery. BullMQ finished **120,372 completed / 6,336 failed** in both.
The 60s burst started at about t=31s, after the first DELETEs were observed at
t=30s. The same expired backlog was therefore being deleted throughout the
burst, unlike the earlier 150k-row test.

The competing writer successfully held 85 transactions for a total of 35.85s
in the baseline run and 86 for 36.34s in phase 1, between roughly t=29s and
t=119s. Mean hold times were 422ms / 423ms (maximum 444ms / 434ms), with
138 / 143 additional acquisition attempts returning busy. No unexpected
external-writer errors were recorded.

**The production-sized buffer overflowed in both builds. Phase 1 is not lossless
under this overload, and these results do not demonstrate fewer channel drops.**
The baseline also failed to consume the entire event stream before shutdown,
so raw drop counters alone are not an equal-progress comparison.

| Measurement | v0.2.1 | Phase 1 |
|-------------|--------|---------|
| BullMQ terminal jobs | 126,708 | 126,708 |
| Terminal events processed at the final pre-shutdown scrape | 88,216 | 126,708 |
| SQLite rows **after shutdown** | 67,170 (63,811 completed / 3,359 failed) | 86,184 (81,881 completed / 4,303 failed) |
| Missing live IDs vs all BullMQ terminal IDs | 59,538, including unread collector backlog | 40,524, exactly matching channel drops |
| Full-channel drop counter | **13,043** | **40,524** |
| Failed write batches | 1 | 0 |
| Explicit error/shutdown-loss records | Not instrumented; cannot isolate from pending work | 0 |
| SQLite retry attempts | Not instrumented | No retry series instantiated; none observed |
| Sampled peak channel depth / capacity | Not instrumented | 16,384 / 16,384 (full); final depth 0 |
| Maximum observed event lag, including final scrape | 442.072s | 97.510s |
| Final most-recent processed-event lag gauge | 442.072s | 43.769s |
| Expired seed rows deleted | 300,000; 700,000 left at shutdown | 1,000,000; none left |
| Non-expired seed rows left | 2,000,000 | 2,000,000 |
| Startup to HTTP-ready, including migration | 0.51s | **16.73s** (building replacement indexes on 3M rows) |
| Fixture-copy time (excluded from startup) | 3.70s | 3.52s |
| Sampled populated FTS-search HTTP mean / maximum | 111ms / 249ms (14 samples) | 234ms / 367ms (34 samples) |
| Console HTML / 1,000-result payload search / FTS integrity | Passed | Passed |

The pre-shutdown baseline persisted-row metric was 63,074; shutdown persisted
another 4,096 rows. Reconciliation initially compared that earlier snapshot
with post-drain counts; the harness assertion was corrected, with a regression
test, to compare **post-shutdown** IDs/states and preserve both snapshots.
Of the baseline's 59,538 absent IDs, 38,492 had not reached the collector's
terminal counter at the final scrape. The remaining difference includes known
channel drops and other observed-but-unpersisted work; the baseline does not
expose enough metrics to split failed-batch records from queued/shutdown losses
precisely. No persisted row had the wrong terminal state in either build.
The baseline summary and existing fixture were reused after the harness fix;
the workload itself was not rerun or silently replaced.

Phase 1 completed two sweep calls and 504 DELETE attempts (including empty and
failed attempts). The first sweep hit the configured 300s deadline after
deleting 958k rows; the next removed the remaining 42k. Their total duration
was 309.41s. Mean DELETE attempt time was **410ms**, excluding in-process
admission; mean admission wait was **286ms for ingestion / 178ms for sweep**.
The corresponding cumulative waits were 94.09s / 89.87s, and DELETE execution
time summed to 206.87s. Mean ingest-batch duration was 576ms in phase 1
(329 batches, including failure semantics) versus 498ms in the baseline
(240 successful batches at the pre-shutdown scrape); these means have different
completion/coverage semantics and are not a normalized throughput comparison.
The external 400ms holds generally completed within SQLite's existing 5s busy
timeout, so contention need not increment the explicit retry counter.

For an equal elapsed-time view, the closest samples to **t=150s** were:

| Counter near t=150s | v0.2.1 (149.91s) | Phase 1 (149.90s) |
|--------------------|-----------------|------------------|
| Collector terminal events processed | 85,713 | 93,628 |
| Persisted observations | 62,047 | 57,007 |
| Channel drops | 13,043 | 21,614 |
| Expired rows deleted | 296,000 | 262,000 |
| Most-recent processed-event lag | 76.749s | 72.146s |

This exposes saturation rather than hiding it behind final totals: phase 1
advanced the collector further but had **more channel drops and fewer persisted
observations at this common time**. Serial admission/pacing, SQLite lock holds
and payload/FTS work still bound the writer. Nothing here justifies claiming
the index change removes that bottleneck or that a larger buffer guarantees
history retention. No smaller-buffer variant was needed to reproduce baseline
drops, and no runtime code was changed based on this experiment.

**Measurement limits:** the baseline's last periodic metric sample was around
153s, but final reconciliation/shutdown did not occur until about 518s. The
available logs do not isolate the source of that large unsampled interval;
do not interpret its wall time or sampled peaks as a complete latency trace.
Its periodic scrapes peaked at 78.668s lag, but the final scrape showed 442.072s.
Phase 1 was sampled through ~349s and finished reconciliation at ~350s.
The intended 420s monitor deadline is not a hard bound on blocking operations
or final queries. Final counters are pre-shutdown; exact final ID/state counts
come from the closed databases. Startup numbers include OS cache/process cost,
not an independently timed index-only build, and do not promise production
migration duration. Both graceful app exits returned 0 and both post-shutdown
FTS external-content checks passed (35.2s / 33.3s).
The event-lag gauge records event age **when an event is processed**, and does
not decay while idle: phase 1's final 43.769s gauge does not mean jobs remained
unconsumed after its terminal counter reached all 126,708 jobs.

Streams contained 514,175 / 514,178 entries, below maxLen=1,000,000;
`entries-added` equalled length, `max-deleted-entry-id` was `0-0`, and the first
entry was `sim-0`: missing rows were not explained by stream trimming. This
remains one sequential run per build on warm local Windows storage/Docker
Valkey, not managed-csi, a randomized benchmark, or a replay of real production
payloads. The diagnostic writer and 2,000 jobs/s burst deliberately exceed the
estimated ~74 completions/s production steady rate.

Runner regression checks:

```powershell
python -m unittest discover -s scripts\sim -p test_compare.py -v
```

### Phase 2 durable-ingest scenarios

With `--scenario normal|restart|trim`, the runner executes only the binary
supplied through the existing `--phase1` argument, labeling it `phase2`.
`--seed` reuses an existing baseline-schema fixture without rebuilding or
rewriting timestamps. These runs use completedTTL=24h so this fixture's retained
tier is still fresh, while its >48h-old expired million remains eligible.
All other buffer/batch/retention/competing-writer settings above are unchanged;
history uses startID=`$`, readBatch=1000 and gapCheckSeconds=1.
The monitor allows up to 650s, and finishes only after the durable cursor reaches
the simulator's final stream tail and the expired backlog drains. Assertions
reconcile exact terminal IDs/states, not just counters.

`restart` kills only the owned binary at approximately t=60s, during the burst,
and restarts it against the same SQLite file while the producer keeps running.
Its pre-kill cursor/counters and restart startup time are recorded separately.
`trim` sets producer maxLen=2000 instead of 1,000,000 and requires a gap signal;
missing records are expected and reported, not hidden by the caught-up cursor.
New `$` queues need the discovery lead-in; use `HISTORY_START_ID=0-0` for an
intentional retained-event backfill.

These are the commands used for the measured runs (from the repository root;
the binaries/fixture were built by the earlier runs):

```powershell
$artifacts = Join-Path $env:USERPROFILE '.copilot\session-state\91103547-9e82-493b-8364-ac38c1e07ca6\files'
docker run --detach --name bdd-history-phase2-31e5b9 --publish 127.0.0.1:16389:6379 valkey/valkey:8-alpine
foreach ($scenario in @('normal', 'restart', 'trim')) {
  $binary = if ($scenario -eq 'normal') { 'phase2.exe' } else { 'phase2-fixed.exe' }
  python scripts\sim\compare.py `
    --output (Join-Path $artifacts 'sim-phase2-3m') `
    --seed (Join-Path $artifacts 'sim-stress-3m\seed.db') `
    --baseline (Join-Path $artifacts 'sim-comparison\baseline.exe') `
    --phase1 (Join-Path $artifacts "sim-comparison\$binary") `
    --container bdd-history-phase2-31e5b9 --scenario $scenario
}
docker stop bdd-history-phase2-31e5b9
docker rm bdd-history-phase2-31e5b9
```

### Measured phase 2, same 3M fixture (2026-10-25/26)

The normal run used `05bb4b1`; restart/trim used `fd9449c`, which fixes
empty-new-stream monitoring noise and discarded-batch telemetry, not the
successful ingest path. A subsequent regression fix also detects completely
emptied/recreated streams (`542ca1e`), and a multi-queue admission regression
(`8c6885a`) passes spare capacity between blocked readers after coalesced
notifications (also on cancellation). Both have focused regression tests;
these three large stress runs were not repeated for those edge cases.
The fixture, host, producer/worker and 126,708-job workload are the same as the
phase 1 stress above. Each copied all 3M rows, including the 1M expired backlog,
and ran actual Bun/BullMQ -> local Valkey -> independent live/history readers ->
SQLite, not a synthetic store-only soak. All three deleted the expired million
and retained the other 2M seed rows. External holds overlapped the burst/sweep:
normal 94 holds/39.60s, restart 100/42.25s, trim 86/36.24s (maximum 431/445/431ms).

| Measurement | Normal | Kill/restart | Small maxLen |
|-------------|--------|--------------|--------------|
| BullMQ completed / failed | 120,372 / 6,336 | 120,372 / 6,336 | 120,372 / 6,336 |
| SQLite completed / failed, exact final IDs/states | **120,372 / 6,336** | **120,372 / 6,336** | 40,328 / 2,124 |
| Missing / unexpected / wrong-state IDs | **0 / 0 / 0** | **0 / 0 / 0** | **84,256 / 0 / 0** |
| Channel drops / explicit error-loss records / failed batches | 0 / 0 / 0 | 0 / 0 / 0 | 0 / 0 / 0 |
| Explicit SQLite retries | 7 ingest retries | None observed | None observed |
| Gap boundaries / partial-record attempts | 0 / 0 | 0 / 0 | **110 / 0** |
| Peak sampled channel depth / capacity | 16,384 / 16,384 | 16,384 / 16,384 | 16,384 / 16,384 |
| Peak committed history lag / final lag | 178.183s / **0** | 144.028s / **0** | 52.987s / 0 (after skipping unavailable events) |
| First sampled caught-up lag + empty channel after workload | t=283.61s | t=242.43s | Not lossless recovery |
| Peak live metrics-reader event lag | 123.159s | 75.841s | 0.378s (many events trimmed) |
| Cumulative enqueue wait | 117.61s | 55.06s | 27.59s |
| Cumulative write admission wait, ingest / sweep | 133.22s / 108.82s | 92.54s / 83.42s | 60.08s / 51.71s |
| DELETE execution sum / attempts | 210.35s / 506 | 212.77s / 503 | 229.31s / 502 |
| Whole-sweep duration sum | 331.85s / 3 calls | 278.30s / 1 completed call; killed call unobserved | 293.71s / 1 call |
| Initial HTTP-ready startup, includes migration/index builds | 20.21s | 20.34s; restart 1.13s | 20.04s |
| Populated search HTTP mean / maximum | 291ms / 515ms | 379ms / 621ms | 163ms / 241ms |
| `/console`, 1,000-result payload FTS search, external-content integrity | Passed | Passed | Passed for surviving rows |

Counters in the restart column add the pre-kill scrape and final process
segments; an abrupt kill cannot export unfinished timings, and scrapes can
miss the last increments. Exact closed-database reconciliation is authoritative.
The killed process had committed cursor `1792973063839-6` at t=60.36s;
the resumed process reached final tail `1792973179060-6`, persisting every
terminal ID with no gap. Its metrics reader intentionally restarted at `$`;
only 89,081 terminal metrics observations were counted across both process
segments, versus 126,708 history rows. Replay is not replayed into metrics.

With the full stream, history lag returned to zero around 125s / 91s after
production ended (rough timings from startup logs; the precise t=283.61 /
242.43 samples above use the runner's monotonic clock). The runner continued
through retention drain to t=371.74 / 368.96s; trim drained by t=324.02s.
Normal DELETE mean was 416ms, excluding admission; ingest/sweep admission
means were 289ms/215ms. Enqueue wait includes cheap successful admissions and
cursor-only checkpoints, not only blocked terminal jobs.
While the normal channel was full at both endpoints, t=99.96--199.77s committed
**672 rows/s**, and t=150.03--249.85s **543 rows/s**. These are observed
100s saturated-window rates with active FTS deletes/lock contention, **not a
hardware-independent ceiling or an isolated maximum-throughput benchmark**.
They are below the 2,000 offered jobs/s burst, so a durable stream backlog is
necessary; the implementation does not make SQLite infinitely fast.

Redis operation histogram counts expose the cost of independent readers:

| Observed operation calls | Phase 1 previous run | Phase 2 normal | Phase 2 restart (segments summed) |
|--------------------------|----------------------|----------------|-----------------------------------|
| Terminal HGETALL | 126,708 | 126,708 history | 143,030 history, including reread of uncommitted work |
| Terminal HMGET | 0 | 126,708 metrics | 89,081 metrics |
| Live XREAD | 8,033 | 8,097 | 6,627 |
| History XREAD / XINFO | Not present | 3,657 / 364 | 3,773 / 367 |

These are instrumented method attempts, not Redis server commandstats (queue
discovery/pipelines/lineage helpers are not all represented); run lengths and
restart live-metric coverage differ. Independence is **not** a guarantee of
low live event lag: the normal metrics reader was slower here than phase 1's
97.510s peak, and the extra hash reads add Redis load. No such latency win is
claimed. In exchange, phase 1 lost 40,524 records to its full channel, whereas
normal/restart phase 2 retained all records and eventually drained.

Trim retained just 2,005 stream entries out of 514,165 emitted (approximate
maxLen=2,000). Valkey's `entries-added > length` detected prefix loss even
though `max-deleted-entry-id` remained `0-0`; logs identify each unavailable
boundary. Both full streams retained all ~514k events. All three final durable
cursors equalled their manifest tail. A zero final lag is **not proof of no
loss** after a gap, and 110 gap boundaries is not a count of 84,256 lost jobs.
Hashes were retained in these stress scenarios, so partial attempts were zero;
the actual Redis integration test independently replays missing terminal hashes,
persists partial rows, resumes a known cursor despite `$`, and detects trimming.

A final small end-to-end check used the final binary after both regression
fixes: BullMQ generated 300 terminal jobs (285 completed / 15 failed), their
300 hashes were explicitly deleted locally, then history started at `0-0`.
All **300 partial rows** were saved, their cursor reached the tail, failed
reasons/returned JSON and stream timestamps survived, state searches returned
285/15 rows, payload FTS for `ok` returned 285, and external-content integrity
passed. Partial attempts were 300, drops zero, and live terminal metrics zero
(historical replay stayed invisible). This exercises missing hashes, not the
producer's automatic remove-policy timing or a multi-queue saturation workload.
Its standalone smoke script/output remain in the session artifacts; the
first HTTP assertion was corrected to supply a required search filter, then
the check passed using a fresh database.

**Limits:** one sequential run per scenario on warm local Windows storage and
Docker Valkey, not managed-csi; deterministic ~1KiB jobs without retries/flows.
Completed TTL differs from the earlier phase 1 run (24h vs 1h solely to keep
the aged retained fixture fresh); both select exactly the same expired million.
Normal producer initialization was several seconds later than scheduled, though
its burst still overlapped the large sweep. The normal build logged four
spurious empty-stream monitoring errors before the first event; the correction
was verified and used for restart/trim. Histogram/counter observations are
attempt-based; 0.5s scrapes miss short peaks. The earlier baseline's large
unsampled interval remains a limitation, not evidence of a phase 2 speedup.
No production volume was used, no alert rules were deployed, and the disposable
container was removed.

## Troubleshooting

- **Cannot connect to Valkey**: ensure Redis is running on `127.0.0.1:6379`.
- **No jobs in dashboard**: refresh and confirm simulator is running.
- **Not seeing waiting-children**: wait for the flow interval (45–90s).
