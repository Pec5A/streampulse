# StreamPulseAuthHighLatency

**Severity**: warning · **Domain**: technical · **Rule**: `deployments/prometheus/alerts.yml`

## What it is
The 95th percentile (p95) response time on `/api/v1/auth/*` exceeds 1 second, for at least 5 minutes straight. Unlike the other two alerts (2-minute window), this one uses a longer window on purpose: latency naturally moves with traffic, and a 30-second blip shouldn't page anyone.

Exact query:
```promql
histogram_quantile(0.95,
  sum(rate(streampulse_http_request_duration_seconds_bucket{path=~".*/api/v1/auth/.*"}[5m])) by (le)
) > 1
```

## Possible causes
1. **Bcrypt cost** — `auth.NewBcryptHasher()` (ADR 0001) is deliberately slow (that's the whole point of password hashing), but a miscalibrated cost factor or high CPU contention on the host can push it further.
2. **Slow DB** — unindexed lookups by email/ID, or Postgres under load.
3. **Noisy neighbor on the compose host** — in a demo environment (a single machine running api+postgres+prometheus+grafana), another container hogging CPU slows everything down. Less relevant once on real infra (ticket K3).
4. **Saturated DB connection pool** — requests queue waiting for a free connection before they even run.

## How to verify
1. Check whether it's DB latency or the bcrypt computation itself:
   ```bash
   docker compose exec postgres psql -U streampulse -d streampulse -c "SELECT * FROM pg_stat_activity WHERE state = 'active';"
   ```
2. Check host/container CPU load:
   ```bash
   docker stats --no-stream
   ```
3. Compare p50/p95/p99 on the Grafana dashboard (`deployments/grafana/dashboards/streampulse-auth.json`, technical panels) — a normal p50 with a spiking p95 points to a handful of slow outlier requests (connection queueing?) rather than a general slowdown.

## What to do
- **CPU saturated by a neighbor** → in local demo, reduce the number of containers running in parallel. On real infra, this is a K3 concern (Kubernetes limits/requests).
- **Slow DB** → check indexes on `users(email)` (already present, see `migrations/0001_create_users.up.sql`); if new queries get added later without an index, that's the first thing to suspect.
- **Saturated pool** → same remedy as `alert-auth-high-error-rate.en.md`: check `pg_stat_activity`, raise `SetMaxOpenConns` only if justified by measured real traffic, not preemptively.
- **Nothing abnormal found** → likely normal variance at low traffic (a single slow call is enough to move a p95 with little volume); compare absolute volume before digging further.
