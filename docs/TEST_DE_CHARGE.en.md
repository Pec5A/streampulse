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

## Two profiles, two different questions

The brief asks both, and neither answers the other:

- "prove the server can absorb **N simultaneous listeners**" → dominated by the **per-subscriber** cost;
- "how much does streaming **100 simultaneous streams** cost us in CPU?" → dominated by the **fixed per-hub** cost.

The harness measures both (`-streams`, `-listeners` per stream).

## Results

Postgres 16 + API in containers (`docker compose`, hardened build), synthetic 128 kbit/s broadcast, 20 s connections. **API restarted before each campaign** — without that, un-collected heap from the previous run skews the baseline, to the point of producing a negative cost.

### Profile A — one stream, many listeners

| Listeners | Connected | TTFB p50 | p99 | Peak heap | Goroutines | Memory / listener | Sustained CPU | Slowest listener | Leak |
|---|---|---|---|---|---|---|---|---|---|
| 1000 | 1000/1000 | 62 ms | 142 ms | 47.0 MiB | 2022 | 45.9 KiB | **0.31 core** | **100%** | +0 |

### Profile B — many streams

| Streams | Listeners/stream | TTFB p50 | p99 | Peak heap | Goroutines | Memory / stream | Sustained CPU | CPU / stream | Leak |
|---|---|---|---|---|---|---|---|---|---|
| 100 | 1 | 39 ms | 47 ms | 22.2 MiB | 320 | 203 KiB | 0.09 core | **0.9 mcore** | +0 |
| 100 | 5 (500 total) | 78 ms | 140 ms | 40.9 MiB | 1121 | 399 KiB | **0.19 core** | **1.9 mcore** | +3 |

## The answer to the brief's question

> "How much does streaming 100 simultaneous streams cost us in CPU?"

**0.19 sustained core** — 19% of a single core — for 100 streams and 500 listeners, in 41 MiB of heap. That is roughly **2 millicores and 400 KiB per stream**.

Put differently: a one-core machine holds this profile with 80% CPU headroom, and memory is not the limiting factor. What binds first remains **outbound bandwidth** — 500 listeners at 128 kbit/s is 8 MB/s sustained, plus 1.6 MB/s inbound for the 100 publishers.

## What the numbers say

**No listener is starved, in either profile.** The slowest listener receives 100% of the expected throughput at 1000 listeners on one stream as well as at 500 listeners spread over 100 streams. That is the core property of the design — non-blocking `Publish` via `select/default` — and it holds in both regimes.

**Per-stream cost is small but not zero.** A hub costs ~200 KiB and ~1 mcore idle, ~400 KiB with 5 listeners. At 100 streams the fixed hub cost (20 MiB) becomes comparable to the subscriber cost — exactly the inversion profile A could not show.

**Latency improves when load spreads.** TTFB p99 is 47 ms at 100 streams × 1 listener against 142 ms at 1 stream × 1000. A hub serving one subscriber has no queue to drain; at 1000, the fan-out loop becomes the hot spot.

**No leak.** Goroutine delta +0 on the first two profiles. The +3 on the third is a measurement artefact, not a leak: the "before" sample was taken right after a restart, at 16 goroutines, while the idle baseline is 19-20 — the three "extra" goroutines are the ones the server normally creates as it leaves cold start.

## Accepted limits

- **Client and server on the same machine.** This measures the hub, not the network: no real loss, latency or congestion. The numbers are a ceiling for the server side, not a production prediction.
- **No scaling beyond 100 streams.** The per-stream cost is measured at 100, not extrapolated past it: the relation could stop being linear once the number of broadcast goroutines exceeds what the scheduler absorbs for free.
- **Heap "after" sometimes exceeds the peak** (41.2 vs 38.5 MiB at 1000). Not a leak — goroutines return to baseline — but a GC that has not run yet: allocated heap stays reserved by the process.
- **Slow listeners are not simulated.** Every client here reads at full speed, so the eviction threshold (64 consecutive drops) is not exercised; the hub's unit tests cover it.

## Reproducing

```bash
JWT_SECRET=load-test-secret-at-least-32-characters docker compose up -d --build

cd backend
# Profile A — one stream, N listeners
go run ./cmd/loadtest -streams 1 -listeners 1000 -duration 20s -ramp-up 8s \
  -metrics-token local-scrape-token

# Profile B — 100 simultaneous streams
go run ./cmd/loadtest -streams 100 -listeners 5 -duration 20s -ramp-up 5s \
  -metrics-token local-scrape-token
```

**Restart the API between campaigns** (`docker compose restart api`): un-collected heap from the previous run skews the baseline. The harness in fact refuses to print a cost when the delta is negative, rather than emitting a confident wrong number.

The harness creates its own broadcaster account and its own stream: no data to prepare. Against a build without `/metrics` it reports client-side numbers only and says so, rather than pretending memory was measured.
