# AGENT.md - Bull-der-dash

This file gives coding agents the minimum context needed to work effectively in `bull-der-dash`.

## What This Project Is
`bull-der-dash` is a Go web app for monitoring BullMQ queues in Redis/Valkey. It serves:
- a live HTMX dashboard
- queue and job inspection pages
- Prometheus metrics
- Kubernetes-friendly health endpoints

The project is optimized for production use, including environments with large BullMQ retention.

## Important Directories
- `main.go`: app bootstrap, HTTP wiring, background refresh loop
- `internal/config`: environment-based config
- `internal/explorer`: Redis access, BullMQ key access, job parsing
- `internal/metrics`: Prometheus metrics
- `internal/web`: handlers, templates, dashboard snapshot cache
- `cmd/redis-cli`: lightweight diagnostic CLI
- `scripts/sim`: Bun simulator for BullMQ load and state coverage

## Current Performance Model
- The dashboard is intentionally live and polled via HTMX.
- Frequently polled views must stay cheap.
- `/queues` should serve from an in-memory snapshot refreshed in the background.
- Fast queue stats should use cheap Redis count operations like `LLEN` and `ZCARD`.
- `/ready` and `/readyz` should stay cheap and use a Redis `PING`, not queue discovery or full stats.
- Expensive diagnostics such as orphaned-job detection are important, but should stay off the hot polling path.

## Practical Rules
- Do not put full queue scans on frequently polled endpoints.
- Be cautious with:
  - `LRANGE ... 0 -1`
  - `ZRANGE ... 0 -1`
  - broad `SCAN` loops
  - repeated per-job state rechecks
- If a feature must remain live, prefer:
  - background refresh
  - in-memory snapshot caching
  - incremental or lazy loading
- Keep search available, but avoid turning it into an unbounded request-time crawl of all retained jobs.

## Redis / BullMQ Notes
Expected BullMQ keys include:
- `bull:{queue}:id`
- `bull:{queue}:wait`
- `bull:{queue}:active`
- `bull:{queue}:paused`
- `bull:{queue}:prioritized`
- `bull:{queue}:waiting-children`
- `bull:{queue}:failed`
- `bull:{queue}:completed`
- `bull:{queue}:delayed`
- `bull:{queue}:stalled`
- `bull:{queue}:{jobId}`

Queue discovery is based on the `:id` keys and the configured `QUEUE_PREFIX`.

## Workflow Expectations
- Prefer `task` commands when the repo already defines them.
- Use `scripts/sim` to generate realistic BullMQ states locally.
- Keep the app shippable as a single binary.
- If static assets move out of inline templates, use `embed`.

## Testing Expectations
- Add or update unit tests for behavior changes where practical.
- Run focused Go tests for touched packages.
- For performance-sensitive work, validate behavior with simulator-driven load where possible.

## Good Next-Step Thinking
When changing the app, ask:
1. Is this endpoint polled or likely to be opened in multiple tabs?
2. Does this code scale with queue count, visible rows, or total retained jobs?
3. Can this move to a cache, background refresher, or diagnostic-only path?
4. Does this preserve the “live dashboard” experience without making Redis do unbounded work?
