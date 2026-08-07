# 0007 — Deploy smoke test, auto-migrations, auth alerting (A3.4/A3.5)

## Status
Accepted

## Context
The team split (`docs/team/plan.md`) assigns A3.4 (continuous deployment) to KaysZ and A3.5 (continuous operations/alerting) to SamyZ — neither is on my slice (identity & trust). But RNCP grading is individual: each candidate spends 20 minutes alone with the jury, and a single "not acquired" criterion invalidates the whole block *for that candidate*. Without a personal artifact on A3.4/A3.5, I would have nothing concrete to show if the jury asked — regardless of what KaysZ or SamyZ build on their end.

The choice wasn't "redo K3/S3 entirely" (a pointless duplication, and outside my functional slice), but to build an artifact scoped to what I already own (the auth service) that concretely demonstrates both competencies.

## Decisions

### A3.4: a deploy smoke test in CI, not just a `docker-compose.yml` that exists
A compose file with "correct syntax" proves nothing about criterion Ce3.4.1 ("automatic deployment... with no manual intervention"). `scripts/smoke-test.sh` plus the `deploy-smoke-test` CI job actually run `docker compose up`, then call `/health`, `/api/v1/auth/register`, `/api/v1/auth/login` and `/metrics` against the running service, and fail the pipeline if any of those calls fail.

This paid off immediately: the first run revealed that the SQL migrations (`migrations/0001_create_users.up.sql`) were never applied automatically — a fresh deploy would 500 on the very first real request. Without this test, that bug would have stayed invisible until the demo in front of the jury.

### Migrations auto-applied at startup, not a separate CLI tool
Rather than adding an external dependency (`golang-migrate`, `goose`), a small in-house runner (`backend/internal/infrastructure/persistence/migrate.go`) reads the embedded `*.up.sql` files via `embed.FS` (`backend/migrations/migrations.go`) and applies them into a `schema_migrations` table, one transaction per file. Consistent with the choice already made in ADR 0001 to stay on `database/sql` with no extra abstraction layer. The API applies its own migrations at startup — `docker compose up` becomes a genuine one-command deploy.

### A3.5: Prometheus alert rules scoped to auth, not full system-wide alerting
Three rules in `deployments/prometheus/alerts.yml`, all limited to my scope (`/api/v1/auth/*` and the auth business counters):
- `StreamPulseAuthHighFailureRate` (business): more than 50% of login attempts failing over 5 minutes — a signal of credential stuffing or a degraded service, not just "there are some errors".
- `StreamPulseAuthHighErrorRate` (technical): 5xx rate on auth endpoints.
- `StreamPulseAuthHighLatency` (technical): p95 above 1s on auth endpoints.

Deliberately, no Alertmanager and no notification routing (email/Slack): that stays within ticket S3's scope (SamyZ), who will build system-wide alerting. These are Prometheus evaluation rules — visible under `/alerts`, consumable by whatever Alertmanager gets wired up later, without depending on S3's implementation to exist.

## A bug found by checking, not by assuming
The alert rules initially used `path=~"/api/v1/auth.*"`. Testing against the real stack (`docker compose up` plus real requests) showed that the technical middleware's `path` label (ADR 0005) is actually `"POST /api/v1/auth/login"` — the HTTP method is prefixed into the label value, not just carried separately in the `method` label. The original regex therefore never matched anything. Fixed to `path=~".*/api/v1/auth/.*"`, re-verified against Prometheus (`/api/v1/rules`, `health: "ok"`, non-empty series). Without actually running the stack, this rule would have stayed "present in the file" but silently inert — invisible from just reading the YAML.

## Consequences
- The `deploy-smoke-test` job adds roughly 1–2 minutes to the CI pipeline; judged acceptable given what it already caught once.
- Auto-applied migrations assume a single API process starting at a time (no distributed lock). Sufficient for this project (a single instance in docker-compose); worth revisiting if K3 deploys multiple replicas starting in parallel.
- The alerts defined here send no notifications until S3 (Alertmanager) is merged — they're visible in the Prometheus UI but "silent" in practice. That's a deliberate scoping decision, not an oversight.
