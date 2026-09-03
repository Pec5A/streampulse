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

## Extension — streaming metrics (September 2026)

The decision above only ever covered authentication. Live broadcasting — the heart of the product — had no business metric at all: we knew an HTTP request answered, not that a listener heard anything. Five metrics were added to the **business** group, in `internal/infrastructure/observability/streaming_metrics.go`:

| Metric | Type | What it answers |
|---|---|---|
| `streampulse_active_streams` | gauge | How many broadcasts are on air right now |
| `streampulse_active_listeners` | gauge | How many people are listening right now |
| `streampulse_broadcast_bytes_total` | counter | Audio volume published since start-up |
| `streampulse_listener_chunks_dropped_total` | counter | Listeners starting to fall behind (leading signal) |
| `streampulse_listener_evictions_total` | counter | Listeners lost for falling too far behind |

Two KPIs were added in a second pass, because the first five did not answer two questions the product asks:

- `streampulse_broadcast_sessions_started_total` — "how many lives happened yesterday?". A gauge structurally cannot answer it: a broadcast that starts and ends between two scrapes never appears in `active_streams`. Counted in the registry and read at scrape time, like the others.
- `streampulse_listener_time_to_first_chunk_seconds` — the wait between a listener attaching and hearing something. That is experienced quality, and it is the only one of the seven that cannot be read at scrape time: it is observed at the event, in the listen handler. It is emphatically not derivable from the HTTP histogram, whose duration for a listen request is the length of the broadcast. It is also the KPI that was missing to prove PR #37's latency fix holds over time.

Three choices shape the implementation:

**Read at scrape time, no hand-maintained counter.** A gauge incremented on subscribe would have to be decremented on all four ways of leaving: unsubscribe, eviction, hub close, process shutdown. Missing one means a gauge that drifts silently for the life of the process. The collector queries the registry at scrape time (`Registry.Totals()`): reading live state cannot drift. As a bonus, `Hub.Publish` — the hot path, once per chunk — is never touched.

**The counters never go down.** When a broadcast ends, its hub dies along with its counters. A total computed only over live hubs would drop, and Prometheus reads a drop as a process restart, corrupting every `rate()` spanning that instant. So the registry absorbs each hub's final counters as it closes (`absorbLocked`). Note the detail that cost us a bug: removing the hub from the map, closing it and absorbing its counters must happen in **one critical section**. Releasing the lock in between opens a window — short but real, reproduced in a test — where the hub is neither live nor retired, and where a scrape reads a counter that went backwards. Covered by `TestTotals_BytesNeverGoBackwardsWhileStreamsClose`.

**No per-stream label.** That would be one time series per broadcast ever started: unbounded cardinality, the classic way to take a Prometheus down. Per-stream detail lives in the traces (ADR 0011), where high-cardinality identifiers are free.

Consequences:

- A second dashboard, `deployments/grafana/dashboards/streampulse-streaming.json` ("Direct (métier)"), 7 panels.
- Two more business alerts (`StreamPulseListenersFallingBehind`, `StreamPulseBroadcastSilent`) with their runbooks — see `docs/runbooks/`.
- `streampulse_broadcast_bytes_total` counts bytes **ingested from broadcasters**, once per chunk regardless of audience. It is not outbound bandwidth: with N listeners, real egress is roughly N times this value. The throughput panels say so explicitly.
- The auth dashboard's HTTP latency panel had to be filtered: `listen`, `publish` and `chat` are HTTP requests whose duration is the length of a broadcast. Unfiltered, they pushed p99 to around ten seconds as soon as a stream was live, making the API look broken.
- The "`/metrics` exposed without authentication" consequence above no longer holds: the endpoint is guarded by `middleware.RequireMetricsToken` (`METRICS_TOKEN`).
