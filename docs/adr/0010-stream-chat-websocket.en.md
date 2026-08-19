# 0010 — Live text chat per stream (ticket Y4, bonus)

## Status
Accepted

## Context
The spec explicitly lists as a bonus feature: *"Websockets: live chat between listeners of the same stream."* This feature already existed in an earlier iteration of the project (`Pec5A/streampulse-reference`, PR #54) — used here only as a reference to understand the approach (hub design, auth choice), not copied: that reference's HTTP framework (`gin`) differs from this repo's (stdlib `net/http`), so the real integration had to be rewritten against K1's actual code (`Hub`/`Registry`, `RequireAuthWS`, `stream_handler.go`), not against the reference.

## Decisions

### `ChatHub` modeled on `Hub`, but N-to-N instead of 1-to-N
The audio hub (`hub.go`, ticket K1) fans one broadcaster out to N listeners. Chat needs the reverse: any participant can publish, and every participant (including the sender) receives every message. `ChatHub` reuses the exact same concurrency design (per-participant channel, non-blocking `select ... default`, context-cancellation-driven close) but with a `Publish` loop that writes to every subscriber rather than one writer feeding N readers.

### No eviction mechanism, unlike `Hub`
`Hub` evicts a listener after 64 consecutive drops (ADR 0008) because a stalled audio listener wastes memory indefinitely until detected. Chat doesn't have that problem at the same scale: a stalled participant loses at most `ChatBuffer` (32) small JSON messages before their WebSocket connection itself eventually times out at the transport level. Adding a dedicated eviction mechanism wouldn't have covered anything normal connection teardown doesn't already handle.

### Room lifecycle tied to the stream's, not managed separately
`StreamUseCase.StartLive` now opens the audio hub **and** the chat room together; `StopLive` and `Delete` close both. A chat room that outlived its stream (or the reverse) would be an inconsistent state that's hard to diagnose. Keeping both in the same method guarantees no future caller can open one without the other.

### `JoinChat` does not require stream ownership, unlike `StartLive`
Publishing audio is broadcaster-only (or admin) — that's `authorise()` in `stream_usecase.go`. Joining chat is open to any authenticated user, the same openness `Listen` has for audio (which doesn't even require an account). Chat only needs an identity to attribute messages to someone — not ownership of the stream. `JoinChat` is therefore a separate method from `StartLive`/`StopLive`, bypassing `authorise()`.

### The displayed username is resolved server-side, never sent by the client
`JoinChat` looks up the real `Username` via `UserRepository.FindByID` once, at connection time. The client's incoming message (`dto.ChatIncoming`) only carries a `text` field — no `username` or `user_id` field is ever read from the incoming WebSocket frame. A malicious client therefore cannot spoof another participant's displayed identity: the username comes from the authenticated session, never from the payload.

### JWT as a query parameter on this route too, via `RequireAuthWS`
Same constraint as `PublishWS` (see ADR 0008): a browser cannot set a header on a WebSocket upgrade request. Chat reuses `middleware.RequireAuthWS` as-is, without duplicating its logic — this is exactly the kind of second use case that middleware was split out of `RequireAuth` for in K1, rather than being folded into it.

### An overlong message is rejected with an error frame, not silently truncated
A message exceeding `maxChatMessageLen` (500 characters, counted in runes via `utf8.RuneCountInString` so multi-byte characters aren't penalized) gets an `{"error": "..."}` frame sent back to the sender alone, and is never broadcast. The connection stays open — this is a rejected message, not a fatal protocol error. Silent truncation (the reference's approach) was ruled out: a user whose message gets cut without knowing it has no way to understand why their listener only sees half a sentence.

### Scope of this ticket: backend only
The mobile chat UI is not included here, consistent with the reference itself, which already documented this deferral ("Mobile interface not included here — a separate ticket if time allows"). This repo's issue #10 only mentions `ChatHub`/`ChatRegistry` + WS endpoint + tests — the backend is the deliverable, defensible on its own at the oral defense; a Flutter screen would remain a separate bonus ticket if time allows before the defense.

## Consequences
- The chat room has no dedicated Prometheus counter (unlike the reference's `streampulse_chat_messages_total`): the `internal/infrastructure/observability` package doesn't exist yet on the K1 branch at the time of this ticket (it lands with ticket Y3, PR #15, not yet merged into `main` at this point). To add once Y3 and K1 are reconciled on `main`.
- This PR is stacked on K1 (`feat/kaysz-streaming-hub`, PR #22, approved but not yet merged) since chat structurally depends on the audio hub and its registry. To rebase onto `main` once K1 merges, following the same mechanics as this project's other stacked PRs.
- Like `Hub`, `ChatRegistry` is in-memory, per-process state: a multi-replica deployment (ticket K3) would need to route a participant to the same instance as the rest of their room — the same limitation already acknowledged for audio in ADR 0008, not a new problem introduced here.
