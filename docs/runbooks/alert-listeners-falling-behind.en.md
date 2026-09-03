# StreamPulseListenersFallingBehind

**Severity**: warning · **Domain**: business · **Rule**: `deployments/prometheus/alerts.yml`

## What it is
More than 3 listeners were disconnected in 10 minutes because they could not keep up with the broadcast, and it has been going on for at least 5 minutes.

This is a **business** alert, and that is the whole point: nothing moves on the technical side. An evicted listener's `GET /api/v1/streams/{id}/listen` request ends normally, with a 200. Neither the 5xx rate nor HTTP latency reacts. This metric is the only place it shows.

Exact query:
```promql
sum(increase(streampulse_listener_evictions_total[10m])) > 3
```

The threshold is a **count**, not a rate. One slow listener on a bad connection is life; a handful in ten minutes is the hub shedding an audience it can no longer serve.

## The mechanism, in two sentences
Every listener has a bounded queue (`Hub.Subscribe`). When a chunk arrives and that queue is full, the chunk is dropped and `streampulse_listener_chunks_dropped_total` goes up. After `maxDrops` consecutive drops the listener is detached and `streampulse_listener_evictions_total` goes up. Drops therefore always precede evictions — they are the leading signal.

## Possible causes
1. **The broadcaster's bitrate outruns what the audience absorbs** — a high-bitrate file put on air. The "Débit de diffusion" panel shows it directly.
2. **The API host is saturated** — fan-out no longer runs fast enough and everyone falls behind at once (check whether evictions arrive in a burst rather than spread out).
3. **A single pathological listener** — a mobile on 3G reconnecting in a loop. Can cross the threshold on its own: check the volume before concluding there is an outage.
4. **Low volume** — in a demo, 4 evictions do not mean much. As with the auth alert, read the threshold against actual traffic.

## How to check
1. See whether drops precede evictions (normal mechanism) or evictions arrive alone (more suspicious):
   ```promql
   rate(streampulse_listener_chunks_dropped_total[5m])
   rate(streampulse_listener_evictions_total[5m])
   ```
2. Put it against the actual audience — 3 evictions out of 4 listeners and 3 out of 400 are not the same story:
   ```promql
   streampulse_active_listeners
   ```
3. Look at throughput at the same instant:
   ```promql
   rate(streampulse_broadcast_bytes_total[1m])
   ```
   If it jumped just before the evictions → cause #1.
4. The metrics carry **no per-stream label** (a deliberate cardinality choice — see ADR 0005). To find out *which* broadcast is at fault, go through the traces in Tempo, where the stream id is available for free.

## What to do
- **Abnormally high throughput** → it is the broadcaster, not the infrastructure. The content on air is too heavy; product-side this is a bitrate cap at upload (not implemented yet, known debt).
- **Evictions in a burst across all listeners** → check host load (`docker stats`, `docker compose logs api`). Fan-out sits in `Hub.Publish`'s hot path; if it slows down, everyone falls together.
- **A single listener, low volume** → false positive, do nothing. If it recurs, the threshold of 3 is probably miscalibrated for this traffic level.
- **Nothing abnormal anywhere else** → note it in the team channel. This alert is new; its threshold has not yet met real traffic.
