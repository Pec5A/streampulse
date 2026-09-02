# Load test — one-to-N broadcasting

> French version: [`TEST_DE_CHARGE.md`](TEST_DE_CHARGE.md).
> Harness: [`backend/cmd/loadtest`](../backend/cmd/loadtest/main.go).
> Closes gap §4.3 of the test plan.

## The question

The brief asks to "prove the server can absorb N simultaneous listeners with minimal memory consumption". The 25-listener end-to-end test proves **correctness** — everyone receives every chunk. That is a different question. Correctness says nothing about latency to first audio under load, about how much memory one more listener costs, or about what happens when a subscriber falls behind.

## Why a Go program rather than k6

The thing under test is a **long-lived chunked HTTP body**. A generic HTTP load tool measures request/response round trips; here the request never ends, and what matters is the byte stream inside it. Go also lets the harness read the server's own `/metrics`: memory is measured **inside the process under test** rather than guessed from outside.

## What is measured

| Metric | Why it |
|---|---|
| Listeners connected / requested | a refused connection is the first symptom of saturation |
| Time to first audio byte (p50/p95/p99) | what a real listener experiences as "how long before I hear anything" |
| Bytes received by the **slowest** listener | the design claims a slow subscriber cannot be starved by its peers — this is the number that checks it |
| Heap at **peak** load | what the machine must hold at its worst moment, not on average |
| Goroutine delta after disconnect | the leak check: every subscription must end with its connection |

## Results

Postgres 16 + API in containers (`docker compose`), synthetic 128 kbit/s broadcast, 20 s connections.

| Listeners | Connected | TTFB p50 | p95 | p99 | max | Peak heap | Goroutines | Cost / listener | Slowest listener | Leak |
|---|---|---|---|---|---|---|---|---|---|---|
| 50 | 50/50 | 54 ms | 99 ms | 104 ms | 106 ms | 11.4 MiB | 122 | 155 KiB | **100%** | +0 |
| 200 | 200/200 | 59 ms | 104 ms | 108 ms | 108 ms | 20.5 MiB | 422 | 73 KiB | **100%** | +0 |
| 500 | 500/500 | 59 ms | 114 ms | 197 ms | 257 ms | 30.9 MiB | 1021 | 23 KiB | **100%** | +0 |
| 1000 | 1000/1000 | 61 ms | 108 ms | 117 ms | 160 ms | 38.5 MiB | 2021 | 12 KiB | **100%** | +0 |

At 1000 listeners, 367 MiB were fanned out in 20 s — roughly 18 MB/s outbound.

## What the numbers say

**No listener is starved, at any level.** The slowest listener receives exactly 100% of the expected throughput, from 50 to 1000. That is the core property of the design — non-blocking `Publish` via `select/default` — and it holds under load, not merely in a 25-listener test.

**Memory does not grow linearly, it amortises.** Cost per listener falls from 155 KiB at 50 listeners to 12 KiB at 1000. The reason: per-subscriber buffers are sized for the worst case (256 chunks × ~4 KB ≈ 1 MB, see ADR 0008), but a listener keeping up leaves its channel nearly empty. **The theoretical worst case is ~1 MB per listener; the measured steady state is 12 KiB.** The gap is the headroom available before a degraded network consumes it.

**No leak.** The goroutine delta is +0 at all four levels: every subscription goroutine ends with its connection. That is what the hub's `context.Context` guarantees, and it is verified rather than asserted.

**Latency stays flat.** p50 moves from 54 to 61 ms between 50 and 1000 listeners. p99 rises to 197 ms at 500 then falls back to 117 ms at 1000 — ramp-window noise, not a trend.

**Sizing.** 1000 listeners fit in **38.5 MiB of heap**. The `shared-cpu-1x` / 512 MB Fly machine in the deployment therefore has ample memory headroom; what binds first is **outbound bandwidth**, not RAM — 1000 listeners at 128 kbit/s is 16 MB/s sustained.

## Accepted limits

- **Client and server on the same machine.** This measures the hub, not the network: no real loss, latency or congestion. The numbers are a ceiling for the server side, not a production prediction.
- **A single stream.** The test loads one live stream with N listeners, which is what the brief asks. N concurrent streams with M listeners each is a different profile, not covered.
- **Heap "after" sometimes exceeds the peak** (41.2 vs 38.5 MiB at 1000). Not a leak — goroutines return to baseline — but a GC that has not run yet: allocated heap stays reserved by the process.
- **Slow listeners are not simulated.** Every client here reads at full speed, so the eviction threshold (64 consecutive drops) is not exercised; the hub's unit tests cover it.

## Reproducing

```bash
JWT_SECRET=load-test-secret-at-least-32-characters docker compose up -d --build

cd backend
go run ./cmd/loadtest -listeners 1000 -duration 20s -bitrate 128 -ramp-up 8s
```

The harness creates its own broadcaster account and its own stream: no data to prepare. Against a build without `/metrics` it reports client-side numbers only and says so, rather than pretending memory was measured.
