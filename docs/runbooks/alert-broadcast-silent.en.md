# StreamPulseBroadcastSilent

**Severity**: warning · **Domain**: business · **Rule**: `deployments/prometheus/alerts.yml`

## What it is
At least one stream has been marked **live** for 10 minutes without a single byte of audio published. Listeners may be connected — they are listening to silence.

This is the classic blind spot of purely technical monitoring: the hub is perfectly healthy, it simply has nothing to forward. No 5xx, no latency, nothing in the logs. The application believes it is broadcasting.

Exact query:
```promql
streampulse_active_streams > 0
and
rate(streampulse_broadcast_bytes_total[5m]) == 0
```

## Why `for: 10m` and not less
The threshold guards against a confusion: a broadcaster holding the publish connection open without sending bytes produces exactly the same signature as a dead broadcast. Ten minutes is the bet — beyond that it is no longer hesitation, it is an abandoned broadcast.

> Checked against the current client: on mobile, stopping the broadcast or changing track closes the publish request body, which triggers `StopLive` and takes the stream offline. Today's client therefore does **not** produce a deliberate silent broadcast — any firing of this alert is abnormal. The long `for` is still warranted: it covers transient network stalls, and a future client that would hold the connection during a pause.

## Possible causes
1. **The broadcaster's upload died without closing the connection** — the most common case. TCP has not given up yet, the server is waiting for bytes that will never come.
2. **A long pause** — a broadcaster leaving the air open while stepping away. Benign, but visible to listeners.
3. **The broadcaster's link dropped** — mobile in a tunnel, Wi-Fi down. Same signature as case 1.
4. **State leak** — a hub left open although the broadcast session has ended. That would be a genuine server-side bug: `StreamUseCase.StopLive` is called from a `defer` in the publish handlers, so there should be no orphan hub. If that is it, this is the serious case.

## How to check
1. Confirm the counter really is flat, and since when:
   ```promql
   increase(streampulse_broadcast_bytes_total[15m])
   ```
   Zero over 15 minutes settles it.
2. Is anyone actually suffering the silence?
   ```promql
   streampulse_active_listeners
   ```
   Zero listeners → embarrassing but harmless. Listeners present → real impact.
3. Ask the API for the list of live streams and check it matches what the metric says:
   ```bash
   curl -s localhost:8080/api/v1/streams/live
   ```
   A stream listed there for a long time that nobody is talking about → orphan-hub candidate (cause 4).
4. The metrics carry **no per-stream label** (ADR 0005): to identify the offending broadcast, cross-reference `/api/v1/streams/live` above, or the Tempo traces where the stream id is present.

## What to do
- **Broadcaster unreachable, listeners present** → end the live server-side to release the listeners rather than leaving them on silence: `DELETE /api/v1/streams/{id}` (owner or admin). `Registry.Close` detaches the listeners and absorbs the hub's counters.
- **Deliberate pause** → do nothing. If it happens often, raise the `for`.
- **Orphan hub suspected (cause 4)** → this is a bug, not an operational incident. Check in the logs that `StopLive` did run for that stream (`docker compose logs api | grep <stream_id>`), and open a ticket. Restarting the API clears everything (`Registry.CloseAll`) but destroys the evidence — capture the logs first.
- **No listeners, no urgency** → note it and watch. This alert is new, its threshold has not yet seen real traffic.
