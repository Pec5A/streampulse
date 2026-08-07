# StreamPulseAuthHighFailureRate

**Severity**: warning · **Domain**: business · **Rule**: `deployments/prometheus/alerts.yml`

## What it is
More than 50% of login attempts (`POST /api/v1/auth/login`) failed (`result != "success"`) over a 5-minute window, for at least 2 minutes straight. This is a **business** alert: it looks at login *outcomes* (wrong password, unknown account...), not HTTP status codes — a server correctly responding 401 to a bad password isn't technically "down", but can still signal a product issue or an attack.

Exact query:
```promql
sum(rate(streampulse_auth_logins_total{result!="success"}[5m]))
/
sum(rate(streampulse_auth_logins_total[5m]))
> 0.5
```

## Possible causes
1. **Credential stuffing / brute force** — someone is testing email/password pairs at scale.
2. **Bad mobile release** — an app release sends a malformed payload (wrong password format, client-side hashing bug).
3. **Upstream DB outage** — `AuthUseCase.Login` reports `result="error"` on internal failures, not just `invalid_credentials`; an unavailable Postgres instance drives up the same counter.
4. **Low traffic** — with little traffic (e.g. demo, dev), a handful of failures is enough to cross 50%; check the absolute volume before treating this as a real anomaly.

## How to verify
1. Check absolute volume, not just the ratio:
   ```promql
   sum(increase(streampulse_auth_logins_total[5m]))
   ```
   If it's 3 attempts with 2 failures, that's not a real anomaly — read the alert in light of actual volume.
2. Split `invalid_credentials` from `error`:
   ```promql
   sum by (result) (rate(streampulse_auth_logins_total[5m]))
   ```
   If `error` dominates → technical outage, see `alert-auth-high-error-rate.en.md`. If `invalid_credentials` dominates → suspected brute force or client bug.
3. Check IP/account diversity if logs allow it (not instrumented yet in this repo — known limitation, see ADR 0007).

## What to do
- **If `error` dominates** → treat as a technical outage (see the error-rate runbook): `docker compose logs api`, check Postgres.
- **If `invalid_credentials` dominates with high volume** → suspected brute force. This repo has no rate-limiting on `/auth/login` yet (known debt, to be addressed with S3 — security). Immediate manual mitigation only: monitor, no automatic blocking available yet.
- **If volume is low** → likely a false positive, no action needed; flag it in the team channel if it recurs often (the 50%/2min threshold may be miscalibrated for a low-traffic environment like the demo).
