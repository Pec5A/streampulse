# StreamPulseAuthHighErrorRate

**Severity**: critical · **Domain**: technical · **Rule**: `deployments/prometheus/alerts.yml`

## What it is
More than 5% of requests to `/api/v1/auth/*` returned a 5xx status over a 5-minute window, for at least 2 minutes straight. Unlike the business alert (login failures), this one only looks at **HTTP status codes** via the technical middleware (`middleware.Metrics`, see ADR 0005) — it fires even if nobody is trying bad passwords, purely because the server is failing.

Exact query:
```promql
sum(rate(streampulse_http_requests_total{path=~".*/api/v1/auth/.*", status=~"5.."}[5m]))
/
sum(rate(streampulse_http_requests_total{path=~".*/api/v1/auth/.*"}[5m]))
> 0.05
```

## Possible causes
1. **Postgres down or unreachable** — this is exactly the bug found while building ADR 0007: without migrations, every real request 500'd. A similar issue (DB down, exhausted pool, wrong `DATABASE_URL`) reproduces the same symptom.
2. **Migrations not applied** — see `persistence.Migrate` in `main.go`; if it fails at startup the API doesn't even start (fatal), so that specific case would look more like a total absence of traffic. But a future badly-written migration could leave the schema in a broken intermediate state.
3. **A bug introduced by a future commit** to `AuthUseCase`/`UserRepository`.
4. **Exhausted DB connection pool** under load (`db.SetMaxOpenConns(10)` in `persistence.Open` — 10 connections max, a traffic spike alone can saturate it).

## How to verify
1. API logs:
   ```bash
   docker compose logs api --tail 100
   ```
2. Postgres health:
   ```bash
   docker compose exec postgres pg_isready -U streampulse
   ```
3. Reproduce locally with the smoke test (`scripts/smoke-test.sh`) — it exercises exactly `/register` and `/login`, so it will reproduce the same 500 if the issue is real and repeatable.
4. Check migrations actually applied:
   ```bash
   docker compose exec postgres psql -U streampulse -d streampulse -c "SELECT * FROM schema_migrations;"
   ```

## What to do
- **Postgres down** → `docker compose restart postgres`, wait for the healthcheck, rerun the smoke test.
- **Pool exhausted** → check active connections (`SELECT count(*) FROM pg_stat_activity;`), raise `SetMaxOpenConns` only if real measured traffic justifies it (not "just in case" — see the project's KISS/YAGNI rule).
- **Broken migration** → never edit a `*.up.sql` file that's already been applied in production; write a new corrective migration (`0002_fix_....up.sql`).
- **Code bug** → roll back the last deploy (`git revert` + redeploy) while investigating, rather than debugging live in prod.
