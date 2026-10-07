# Grafana dashboard pack

Ready-to-import Grafana dashboards for bull-der-dash, plus a self-contained
local stack (Prometheus + Grafana) that provisions them automatically.

Everything here reads **only** metrics bull-der-dash already exports
(`internal/metrics/metrics.go`) — there are **no code changes** required to use
it. The workload dashboards light up when `WORKLOAD_METRICS_ENABLED=true`.

## Dashboards

| File | UID | What it shows |
|------|-----|---------------|
| `dashboards/queue-health.json` | `bdd-queue-health` | Fleet summary stats, per-queue backlog (waiting / active / delayed / prioritized / waiting-children), failure & stall watch, and an instant state-breakdown table. Driven by the `bullmq_queue_*` gauges. |
| `dashboards/workload-throughput.json` | `bdd-workload` | Completed vs failed throughput, failure ratio, p50/p95/p99 completion latency + a duration heatmap, per-job-name breakdown, and event lag. Driven by the event-stream `bullmq_jobs_finished_total` / `bullmq_job_completion_duration_seconds` series. |
| `dashboards/collector-internals.json` | `bdd-internals` | Collector health (events read/dropped, lookup errors, event lag), Redis op latency/errors, HTTP rate/latency, and process memory/goroutines/CPU. |

All three expose a **`datasource`** variable (pick any Prometheus) and a
**`queue`** filter; the workload board adds a **`name`** (job-name) filter.

## Quick start (local)

1. Run bull-der-dash with workload metrics on:

   ```bash
   task dev   # serves the dashboard + /metrics on :8080, WORKLOAD_METRICS_ENABLED=true
   ```

2. Bring up Prometheus + Grafana (auto-provisioned):

   ```bash
   docker compose -f grafana/docker-compose.yaml up -d
   ```

3. Open Grafana at <http://localhost:3001> (admin / admin). The dashboards are
   already under the **Bull-der-dash** folder; "Queue Health" is the home
   dashboard. Prometheus is at <http://localhost:9491>.

Ports (3001 / 9491) are intentionally non-default so this stack coexists with
other local Grafana/Prometheus instances. Prometheus scrapes the host via
`host.docker.internal:8080`; edit `prometheus/prometheus.yml` to point at a
different target (e.g. an in-cluster Service).

## Using in an existing Grafana

- **Import:** Dashboards → New → Import → upload a file from `dashboards/`, then
  pick your Prometheus datasource when prompted.
- **Provision (Helm / sidecar):** the JSON files are standard provisioning
  payloads. With the Grafana dashboard sidecar, load each file from a ConfigMap
  labelled for discovery; the `datasource` variable resolves to your default
  Prometheus automatically.

## Alerts

`alerts/queue-pressure.yaml` is a Grafana unified-alerting provisioning file
(source of truth) with two per-queue rules that pair with the Queue Health
**Backlog pressure** panels:

| UID | Fires when | Default |
|-----|-----------|---------|
| `bdd-drain-time-default` | `waiting / completed-per-sec` (drain ETA) stays high | > 900s for 10m |
| `bdd-backlog-growing-default` | `deriv(bullmq_queue_waiting[15m])` stays positive | > 0.05 jobs/s for 15m |

Both are multi-dimensional (one alert instance per `queue`, via `legendFormat
{{queue}}`) and ship with no `notification_settings`, so they attach to your
default notification policy. Thresholds are deliberately conservative — tune
`for` and the threshold `params` to your SLOs. Drop the file alongside the
dashboards wherever you provision Grafana; downstream deployments (e.g. a
cluster ConfigMap) can add a cluster label prefix and a specific receiver.

## Notes

- **Failed / stalled / orphaned** stat tiles turn red at the first non-zero
  sample — they are meant to be glanceable alarms.
- Workload panels are **event-derived**: they count jobs observed finishing
  after the collector started (`WORKLOAD_METRICS_START_ID=$` by default) and do
  not backfill already-retained jobs.
- `name` cardinality is capped per queue by
  `WORKLOAD_METRICS_MAX_JOB_NAMES_PER_QUEUE`; overflow shows as `__other__`,
  and terminal events whose job hash is already gone show as `__unknown__`.
