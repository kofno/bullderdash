# GEMINI.md - Bull-der-dash

Foundational mandates for Gemini CLI in the `bull-der-dash` repository.

## Project Context
`bull-der-dash` is a high-performance dashboard for monitoring BullMQ queues, built with Go and Prometheus. It is designed for Kubernetes-native deployments, production safety, and low overhead under large retained-job counts. The live dashboard is HTMX-driven, while the newer full-text search console (`/console`) is a dependency-free vanilla-JS page backed by an embedded SQLite history store; HTMX is being phased out in favor of the console.

## Core Mandates

### Architecture & Conventions
- **Go Standard Library First**: Favor the Go standard library for HTTP routing (`http.ServeMux`) and templating (`html/template`).
- **Single Binary Mandate**: The application MUST be shippable as a single executable.
  - If HTML, CSS, or other assets are moved out of strings into separate files, they MUST be bundled using the `embed` package.
- **Package Structure**:
  - `internal/config`: Environment-based configuration.
  - `internal/explorer`: Redis/Valkey interaction and BullMQ parsing logic.
  - `internal/metrics`: Prometheus metric definitions.
  - `internal/store`: Embedded SQLite job-history store with FTS5 full-text search.
  - `internal/workloadmetrics`: BullMQ event-stream collector feeding metrics and history persistence.
  - `internal/web`: HTTP handlers, templates, lightweight view caches, and the embedded search console.
- **Operational Shape**:
  - Keep the dashboard live, but keep request paths cheap.
  - Prefer background refresh plus in-memory snapshots for frequently polled UI fragments.
  - Readiness and liveness endpoints must stay cheap and Kubernetes-safe.
- **Persistence & Search**:
  - Job history is persisted to an embedded SQLite database (`modernc.org/sqlite`, pure-Go, no CGO, WAL mode) from a background writer — never on the live polling path.
  - Full-text search is served from SQLite FTS5 via `/console` and `/v1/search`; it must not fall back to scanning live Redis.
  - SQLite is a single writer: the deployment runs a single replica with a `ReadWriteOnce` volume and the `Recreate` update strategy.
- **Frontend**:
  - The live dashboard uses **HTMX** with **Tailwind CSS** classes directly in templates.
  - The search console is dependency-free vanilla JS embedded via `go:embed`; prefer the console for new UI work and avoid adding new HTMX/CDN dependencies.
- **CLI Helper**: The `cmd/redis-cli` tool is a lightweight helper for Redis operations; keep it focused on diagnostics.

### Performance Rules
- **Hot paths must be bounded**:
  - The live dashboard (`/queues`) must avoid per-request scans over all retained jobs.
  - Prefer `LLEN`, `ZCARD`, and other cheap cardinality operations for live counts.
  - Expensive diagnostics such as orphaned-job detection should not run on the dashboard polling path.
- **Polling is allowed**:
  - Preserve live HTMX polling where it adds value.
  - Make polled endpoints serve cached snapshots or similarly cheap summaries.
- **Diagnostics are separate**:
  - Deep scans, orphaned detection, and other expensive correctness checks belong in admin/diagnostic flows or background work.
- **Search should remain useful**:
  - Full-text search is served from the embedded SQLite FTS5 history store, not from live Redis.
  - Persistence happens on a background writer; keep it off the live dashboard polling path.
  - Never regress search into an unbounded request-path Redis crawl.

### BullMQ / Redis Assumptions
- Queue discovery is based on BullMQ key patterns like `bull:{queue}:id`.
- Queue counts should rely on the native BullMQ lists and sorted sets:
  - `wait`, `active`, `paused`
  - `prioritized`, `waiting-children`, `failed`, `completed`, `delayed`, `stalled`
- Job detail and search paths may be more expensive than dashboard counts, so optimize them separately from the live dashboard path.

### Development Workflow
- **Taskfile**: ALWAYS use `task` for common operations when available.
  - `task build-all` to build both dashboard and CLI.
  - `task dev` for local development.
- **Simulation**: Use `scripts/sim` (Bun/TypeScript) to generate BullMQ data and noise in Redis for local development.
- When validating performance changes, prefer simulator-driven tests that mimic production retention and polling behavior.

### Testing & Quality
- **Test Mandate**: New logic or behavior changes should include unit tests (`*_test.go`) where practical.
- **Standards**: Adhere to `go fmt` and `go vet`.
- **Performance-sensitive changes**:
  - Verify that the live dashboard path remains cheap under larger retained job counts.
  - Watch for accidental reintroduction of full `LRANGE 0 -1`, `ZRANGE 0 -1`, or broad `SCAN` behavior on frequently polled endpoints.
- **Mocking**: Use standard library testing. Consider `miniredis` for Redis-related tests if needed, but verify fit before adding it.

### Security
- **Credentials**: Never log or hardcode Redis passwords or sensitive configuration.
- **Environment**: Use `.env.example` as a template for local environment variables.

## Technical Stack
- **Backend**: Go 1.26+
- **Live Data**: Redis / Valkey (via `go-redis/v9`)
- **History & Search**: Embedded SQLite with FTS5 (`modernc.org/sqlite`, pure-Go, CGO-free)
- **Frontend**: HTMX + Tailwind CSS (live dashboard); dependency-free vanilla JS (search console)
- **Observability**: Prometheus
- **Environment**: Kubernetes (kinD for local dev)
