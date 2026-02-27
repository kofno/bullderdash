# GEMINI.md - Bull-der-dash

Foundational mandates for Gemini CLI in the `bull-der-dash` repository.

## Project Context
`bull-der-dash` is a high-performance, stateless dashboard for monitoring BullMQ queues, built with Go, HTMX, and Prometheus. It is designed for Kubernetes-native deployments and efficiency.

## Core Mandates

### Architecture & Conventions
- **Go Standard Library First**: Favor the Go standard library for HTTP routing (`http.ServeMux`) and templating (`html/template`).
- **Single Binary Mandate**: The application MUST be shippable as a single executable. 
  - If HTML, CSS, or other assets are moved out of strings into separate files, they MUST be bundled using the `embed` package.
- **Package Structure**: 
  - `internal/config`: Environment-based configuration.
  - `internal/explorer`: All Redis/Valkey interaction and BullMQ parsing logic.
  - `internal/metrics`: Prometheus metric definitions.
  - `internal/web`: HTTP handlers and embedded HTML templates.
- **Statelessness**: The application MUST remain stateless. All queue data must be fetched from Redis/Valkey on-demand.
- **Frontend**: 
  - Use **HTMX** for dynamic updates (e.g., periodic queue list refreshes).
  - Use **Tailwind CSS** classes directly in HTML templates for styling to maintain ease of packaging.
- **CLI Helper**: The `cmd/redis-cli` tool is a lightweight helper for Redis operations; keep it focused on diagnostics.

### Development Workflow
- **Taskfile**: ALWAYS use `task` for common operations (build, dev, simulation). 
  - `task build-all` to build both dashboard and CLI.
  - `task dev` for local development.
- **Simulation**: Use `scripts/sim` (Bun/TypeScript) to generate test data/noise in Redis for local development.

### Testing & Quality
- **Test Mandate**: Any new logic or features MUST be accompanied by unit tests (`*_test.go`).
- **Standards**: Adhere to `go fmt` and `go vet` for code quality and formatting.
- **Mocking**: Use standard library testing or consider `miniredis` for Redis-related unit tests if needed (verify usage before adding).

### Security
- **Credentials**: Never log or hardcode Redis passwords or sensitive configuration.
- **Environment**: Use `.env.example` as a template for local environment variables.

## Technical Stack
- **Backend**: Go 1.25.4+
- **Database**: Redis / Valkey (via `go-redis/v9`)
- **Frontend**: HTMX, Vanilla HTML/Templates, Tailwind CSS
- **Observability**: Prometheus
- **Environment**: Kubernetes (kinD for local dev)
