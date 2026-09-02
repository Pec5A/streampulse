# 0005 — Observability: separating business and technical metrics (ticket Y3)

## Status
Accepted

## Context
The spec explicitly asks to "distinguish, on a Grafana dashboard, 500 errors (technical) from abrupt user disconnections (business/experience)". We had to decide how to structure Prometheus metrics so that this distinction is real in the code, not just in how the dashboard is laid out.

## Decision
Two metric groups, named and commented as such in `internal/infrastructure/observability/metrics.go`:

- **Business** (`streampulse_auth_logins_total`, `streampulse_auth_registrations_total`): incremented inside `AuthUseCase`, the layer that knows the *business meaning* of a failure (wrong password ≠ email already taken ≠ server error). Labelled by outcome (`success`/`invalid_credentials`/`error`, `success`/`conflict`/`error`).
- **Technical** (`streampulse_http_requests_total`, `streampulse_http_request_duration_seconds`): incremented by a generic middleware (`middleware.Metrics`) that only knows the route and the HTTP status code — no business logic involved.

The dashboard (`deployments/grafana/dashboards/streampulse-auth.json`) has separate panels for each group: logins/registrations by outcome (business) on one side, 5xx error rate and p50/p95/p99 latency (technical) on the other.

## Consequences
- `/metrics` is exposed without authentication for now (local scraping via docker-compose). Needs securing if the service is ever exposed publicly (ticket S3).
- The docker-compose stack (api + postgres + prometheus + grafana) has since been exercised with a real `docker compose up` (see ADR 0007): that run revealed the SQL migrations were never applied automatically, fixed in the same batch of work. Without that real test, this bug would have stayed invisible until the demo.
- The docker-compose file will likely be extended by ticket K3 (full deployment, K8s) — expected collaboration on `deployments/`.
