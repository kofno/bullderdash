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

## Troubleshooting

- **Cannot connect to Valkey**: ensure Redis is running on `127.0.0.1:6379`.
- **No jobs in dashboard**: refresh and confirm simulator is running.
- **Not seeing waiting-children**: wait for the flow interval (45–90s).

