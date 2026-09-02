# 0008 — Live broadcasting architecture (ticket K1)

## Status
Accepted

> Number 0008: ADRs 0002 to 0007 were already claimed by open PRs (playlists,
> admin, security CI, observability, GDPR, smoke test). K1 takes the first
> free number so it stays mergeable whatever order the PRs land in.

## Context

K1 requires a *broadcaster* to send a live audio stream to N simultaneous
*listeners*, with bounded memory use, from a mobile app and from a browser.
Three things had to be decided: how to carry the audio, how to distribute it
in memory, and how to play it back on the Flutter side.

## Decisions

### In-memory fan-out: one hub per stream, one goroutine per listener

`Hub` (`internal/infrastructure/streaming/hub.go`) holds a
`listenerID -> chan []byte` map. The broadcaster calls `Publish`, which copies
the chunk **once** and then pushes it into every channel with
`select ... default`.

Three properties follow:

1. **`Publish` never blocks.** A slow listener cannot slow down the
   broadcaster or its peers — the `default` clause is what guarantees it.
2. **Memory is bounded by construction**: `listeners × 256 chunks × ~4 KB`,
   so roughly 1 MB per listener at worst. Nothing grows without a ceiling.
3. **No goroutine leaks**: closing the hub closes every channel, so each
   listener handler falls out of its loop. A test checks it
   (`TestHub_DoesNotLeakGoroutines`).

**Why one copy per publish and not per listener**: the broadcaster's handler
reuses a single read buffer. Without a copy, a slow listener would read an
array that the next read had already overwritten. Copying once per publish
(rather than once per listener) makes the chunk immutable for everyone at
minimal cost. Covered by `TestHub_PublishCopiesTheCallersBuffer`.

### Eviction after 64 consecutive dropped chunks, not after the first

The reference implementation detached a listener as soon as one chunk was
dropped. That is too aggressive: a network latency spike of a few hundred
milliseconds disconnects a perfectly healthy listener.

Here the drop counter is **consecutive** and reset on every successful send.
Transient jitter resolves on its own; only a genuinely dead listener (closed
TCP connection, suspended app) reaches 64 drops in a row and is evicted to
free its memory.

### The hub knows nothing about Prometheus or logging

The hub exposes `Stats()` (listeners, bytes broadcast, dropped chunks,
evictions) and nothing else. It is the transport layer — or ticket Y3's
Prometheus collector — that decides what to record.

Direct consequence: the package is tested with the standard library alone, and
it does not depend on the `observability` package introduced by a PR that is
not merged yet.

### Per-process registry (an accepted limitation)

`Registry` indexes the hubs alive in the current process. A multi-replica
deployment would therefore require listeners to reach the same replica as
their broadcaster (sticky sessions), or a real message bus (Redis pub/sub,
NATS).

This is a deliberate choice at this project's scale: the complexity of an
external bus is not justified here, and the decoupling point is already
identified should it become so — `Registry` is the single entry point.

### Two publish paths: chunked HTTP **and** WebSocket

- `POST /api/v1/streams/{id}/publish`: request body in
  `Transfer-Encoding: chunked`, read continuously. This is the native clients'
  path.
- `GET /api/v1/streams/{id}/publish/ws`: binary WebSocket frames. **A browser
  cannot stream an HTTP request body** — that is the only reason this second
  path exists.

Both feed the same hub, so listeners see no difference.

### The `publish` handler does not answer 200 before the end

A non-obvious point, discovered while writing the end-to-end test: a
conforming HTTP client **stops sending the request body as soon as a response
arrives** (Go's `http.Transport` does exactly that). Acknowledging the publish
straight away therefore cut the broadcast off at the first chunk.

The response is now the *result* of the session (bytes broadcast), written
once broadcasting has ended. Authorisation failures (401/403/404) still answer
immediately — which is precisely what tells a rejected broadcaster to stop
sending.

### The JWT in the query string, on the WebSocket route only

The browser WebSocket API does not allow adding a header to the upgrade
request. `middleware.RequireAuthWS` therefore accepts the token either in
`Authorization` or in `?token=`.

This is strictly worse than a header: a URL ends up in access logs, proxy logs
and browser history. The mitigation is **containment**: this middleware is
mounted on that route only, and every other authenticated route stays
header-only. A test locks it down
(`TestRequireAuth_DoesNotAcceptAQueryToken`) — if it ever breaks, it means the
whole API has started accepting credentials in URLs.

Other mitigations: short-lived tokens (24 h), TLS mandatory in production, and
the value is never logged by our handlers.

### `ListLive` reconciles the database with the registry

If the API restarts during a broadcast, the row stays `live` in the database
while the hub is gone. `ListLive` therefore filters on `registry.IsLive`, so a
listener is never offered a live stream that would 404 the moment they tapped
it.

### Mobile: an `AudioEngine` port in front of `just_audio`

`just_audio`'s `AudioPlayer` goes through method channels: a bloc that depends
on it can only be tested with a real engine behind it. The player therefore
depends on an `AudioEngine` interface (~10 methods), implemented in a single
file by `JustAudioEngine`.

Concrete benefit: **the whole state machine — including interruption handling,
the part that is hardest to reproduce on a real phone — is covered by fast
unit tests** (incoming call that pauses playback, resuming only when the OS
asks for it, headphones unplugged). 24 tests, no device needed.

### One object for background playback and for the UI

`StreamAudioHandler` is both a `BaseAudioHandler` (the OS side: lock screen,
notification) and an `AudioEngine` (the app side: the bloc). A pause from the
lock screen and a pause from the app therefore take exactly the same path —
there are not two state machines to keep in sync.

### Seeking is refused on a live stream, not hidden

A live stream has no past to go back to: `duration` is `null`, `canSeek` is
false, and the bar is replaced by a "Live" indicator. The control stays where
the user expects it and explains itself, instead of disappearing.

Seeking remains implemented and tested (clamped at both ends): it will serve
ticket K2's recorded tracks.

## Consequences

- `internal/infrastructure/persistence/stream_repository.go` has no unit tests
  — it needs a real Postgres. Same limitation as `user_repository.go`, already
  noted in ADR 0001; to be covered with testcontainers in a dedicated ticket.
- One new backend dependency: `github.com/coder/websocket` (minimal, no
  transitive dependencies).
- Three new mobile dependencies: `just_audio`, `audio_service`,
  `audio_session`, plus the platform configuration that goes with them
  (Android service and permissions, `UIBackgroundModes: audio` on iOS).
- Multi-replica broadcasting is out of scope for as long as the registry lives
  in memory (see above).
