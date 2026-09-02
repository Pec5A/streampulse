# 0012 — Hardening before going to production (ticket Y5)

## Status
Accepted

## Context
The delivery pipeline (ticket K3) makes the service **publicly reachable**. A review run just before enabling it found four gaps against explicit requirements of the brief — "stream security (TLS), injection protection and **securing sensitive endpoints (admin/metrics)**", and "zero hardcoding":

1. **No CORS**, while CD publishes a `flutter build web` artefact. A page served from another origin has every request blocked by the browser: the artefact ships but cannot be used.
2. **No rate limiting anywhere.** `POST /api/v1/auth/login` accepts unlimited attempts; bcrypt makes each one slow, nothing made them few.
3. **`/metrics` open.** The code comment justified it as *"acceptable for now since nothing here is deployed publicly yet (ticket K3)"*. Ticket K3 is exactly the one that deploys — the justification expired at the next merge.
4. **`JWTExpiration` hardcoded** at 24 h, in the file whose header states that nothing is.

## Decision

### CORS: exact allowlist, never a reflection
The incoming origin is compared against a list configured by `CORS_ALLOWED_ORIGINS`. It is **never echoed back**. Reflecting `Origin` is the classic way to end up with an API that trusts every site on the web: it is silently equivalent to `*`, except that — unlike `*` — it also works with credentials, which is precisely what makes it dangerous.

An empty list means **no CORS headers at all**. That is the right default for an API whose only client is a native app: browser access is opt-in, never implicit.

Preflights are answered by the middleware rather than passed down: the mux returns `405` for an `OPTIONS` on a route registered as `POST`, and the browser reads that 405 as a denial.

### Rate limiting: in-memory, per IP, owned as such
20 requests/minute per IP on `/auth/*` (`AUTH_RATE_LIMIT_PER_MINUTE`). Deliberately **in-memory and per instance**: a shared limiter (Redis) is the right answer once there is more than one machine — with N instances the effective limit is N times this one. Choosing the simple version now is a trade-off, not an oversight: it removes the trivial attack today without adding an external dependency the project does not otherwise need.

**Client identification goes through `TRUSTED_PROXY_HOPS`**, because the two naive options are wrong in opposite directions:

- *Always use `RemoteAddr`.* Behind a TLS-terminating proxy — which every PaaS is — that address is the proxy's, identical for everybody. The quota then applies to **all users combined**: with a 20/min limit, the 21st login attempt of the minute fails no matter who makes it. An earlier version of this ADR called that "conservative" and claimed it "under-counts nobody". It was the opposite: pooling every client into one bucket **over-counts** each of them, and turns the protection into a self-inflicted denial of service the first time a few people sign in at once. Caught in review by @monkeyDkz.
- *Always trust `X-Forwarded-For`.* Any client can send that header, so an attacker mints a fresh quota per request by varying it. The limiter becomes decorative.

**Counting from the right** is what makes the header usable: an attacker can prepend entries but cannot remove the ones the infrastructure appends after theirs. Skipping exactly `TRUSTED_PROXY_HOPS` entries from the end therefore lands on the address the closest trusted proxy actually observed. The hop count is deployment configuration — 0 locally, 1 behind a single PaaS load balancer — and defaults to 0: a shared quota is bad, a forgeable one is worse.

The table is swept on every request to forget quiet clients. Without that it grows by one entry per distinct IP that ever touched the service and never shrinks — the same unbounded-growth shape as the Prometheus label cardinality problem, reached from another direction.

### `/metrics`: bearer token, and a refusal to start without one
`/metrics` is not innocuous: it publishes the route table, per-route volumes, latency distributions and the process memory profile. That is a map of the application handed to whoever asks.

Guarded by a token (`METRICS_TOKEN`), compared in **constant time** — a byte-by-byte comparison leaks the token one character at a time to anyone willing to measure. The failure response is **404, not 401**: an unauthenticated scanner learns nothing, not even that the endpoint exists.

`config.Load` **refuses to start** when `ENVIRONMENT` is not `development` and the token is missing. Failing loudly at boot is the only way this cannot be forgotten on the day it starts mattering — the day the service goes live.

### Security headers
`nosniff`, `X-Frame-Options: DENY`, `Referrer-Policy: no-referrer`, and a `default-src 'none'` CSP — the API returns only JSON, so forbidding every source is accurate rather than merely restrictive.

HSTS **only outside development**: sent from a plain-HTTP local server it would pin `localhost` to HTTPS in the developer's browser, for a year, across every project on that port.

### `JWT_EXPIRATION` from the environment
Default 24 h, validated (Go duration, strictly positive). Beyond 12-Factor, this value has a security consequence: it is how long a token stays usable **after** the account it names is deleted — an operator must be able to shorten it without rebuilding the image.

## Consequences
- **The local stack exercises the production path.** `METRICS_TOKEN` is set in compose and Prometheus presents it, even though development would tolerate an empty one. A guard proven only by unit tests is a guard discovered broken in production.
- **Prometheus trap**: its configuration does **not** expand environment variables. A `${METRICS_TOKEN}` there would be sent verbatim and every scrape would 404 — silently, since a failing scrape only shows as a gap in the graphs. The local value is therefore literal and commented as such; a deployment supplies its own via `credentials_file`.
- **Coupling with the deployment**: with `ENVIRONMENT=production` the API **will not start** without `METRICS_TOKEN`. The secret must exist before the first deploy, otherwise the health check fails and Fly keeps the previous version — intended behaviour, but it belongs in the runbook.
- **Not addressed**: token revocation (a revocation list needs shared storage), distributed rate limiting, and an admin token distinct from the metrics one.

## Verified
Full stack brought up, raw results:

```
/metrics without token       404
/metrics with token          200 (66 series)
headers                      nosniff, DENY, no-referrer, CSP present
CORS allowed origin          Allow-Origin + Vary: Origin
CORS rejected origin         0 CORS headers
rate limit /auth/login       20 x 401 then 429
Prometheus target            health=up
```
