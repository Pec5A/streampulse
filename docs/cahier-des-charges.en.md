# Specification — StreamPulse

> French version: [`cahier-des-charges.md`](cahier-des-charges.md).
> Semester project, 5A Tech Lead — S2, Block 3 of RNCP 38822, École .decode.
> Team: Yassir Sabbar ([@JASSBR](https://github.com/JASSBR)), KaysZ ([@monkeyDkz](https://github.com/monkeyDkz)), SamyZ ([@SamyNikaia](https://github.com/SamyNikaia)).

---

## 1. Context and objective

Live streaming demands infrastructure able to fan a single stream out to many recipients with low latency and controlled resource use. StreamPulse is a **real-time audio broadcasting platform**: one *broadcaster* emits a stream, N *listeners* receive it simultaneously.

The teaching objective is not the feature alone but **how it is industrialised**: resilience, observability, automation. The project is assessed against Block 3 of RNCP 38822 ("Driving the production release of software solutions and their evolution").

## 2. Scope

### In scope

Live audio broadcasting is the central feature; everything else exists to make it usable and operable.

| Area | Content |
|---|---|
| Identity | registration, login, session refresh, roles |
| Broadcasting | starting a stream, publishing a feed, listening by N listeners |
| Content | playlists with an ordered queue, audio track upload |
| Administration | role management, user listing, global statistics |
| Compliance | personal data export, account deletion |
| Operations | metrics, traces, correlated logs, alerts, dashboards |
| Delivery | continuous integration and deployment, production release |

### Out of scope, deliberately

- **Adaptive transcoding**: audio bytes are multiplexed as-is, never re-encoded. The format is the broadcaster's decision.
- **Recommendation**: no suggestion algorithm.
- **Multi-instance broadcasting**: the hub is in memory, therefore bound to one process. Moving to several instances would require an external bus — a decision documented in ADR 0008, not an oversight.
- **Token revocation**: a JWT stays valid until it expires, including after the account is deleted. Its lifetime is configurable so that window can be bounded (ADR 0012).
- **Payment, subscription, content moderation.**

## 3. Roles and permissions

| Role | Can |
|---|---|
| **Anonymous** | browse live streams, listen to a stream |
| **User** | + manage their account, create and order playlists, export or delete their data |
| **Broadcaster** | + start a stream, publish a feed, upload tracks |
| **Admin** | + list users, change roles, view global statistics |

Listening is **public by choice**: requiring an account to listen to a live radio would be friction with no upside, and it keeps the authenticated attack surface smaller.

## 4. Functional requirements (user stories)

Each story carries its acceptance criteria. The tests verifying them are named in the [acceptance test book](CAHIER_DE_RECETTE.en.md).

### 4.1 Identity and account

**US-01 — As a visitor, I want to create an account, so that I can listen and build playlists.**
- 201 and account created; password stored hashed (bcrypt), never in clear
- 409 if the email already exists, with no second account created
- 400 if the password is too short

**US-02 — As a user, I want to log in, so that I can find my content again.**
- 200 and a token usable on protected routes
- an identical 401 whether the email is unknown or the password wrong: the API must not reveal which accounts exist
- beyond 20 attempts per minute per IP: 429

**US-03 — As a user, I want to extend my session without re-entering credentials.**
- 200 and a new token; 401 without a valid token

### 4.2 Live broadcasting

**US-04 — As a broadcaster, I want to open a live stream, so that listeners can join me.**
- 201, status `live`, the stream appears in the live listing
- only the owner can publish to it or stop it

**US-05 — As a listener, I want to listen without creating an account.**
- 200 and a chunked stream, unauthenticated
- audio starts in under a second under nominal conditions

**US-06 — As an operator, I want N listeners to receive the same stream without interfering with each other.**
- a slow listener slows neither the broadcaster nor its peers
- a genuinely dead listener is evicted after 64 **consecutive** drops, not on the first: network jitter must not disconnect a healthy client
- **verified**: 1000 simultaneous listeners, the slowest receiving 100% of the expected throughput ([load test](TEST_DE_CHARGE.en.md))

**US-07 — As a browser-based broadcaster, I want to publish from a web page.**
- a browser cannot stream an HTTP request body: a dedicated WebSocket route exists for that case

### 4.3 Content

**US-08 — As a user, I want to organise my tracks into playlists.**
- full CRUD, restricted to the owner (403 otherwise)
- a private playlist is not visible to a third party

**US-09 — As a user, I want to reorder my queue.**
- the new order is persisted in one transaction
- an order that is not an exact permutation is rejected with no partial change
- removing a track leaves no hole in the positions

**US-10 — As a broadcaster, I want to upload an audio file.**
- atomic upload: an interrupted transfer leaves neither a final nor a temporary file
- anything that is not binary is rejected — audio is never text
- playback supports `Range` requests (resuming mid-track)

### 4.4 Administration

**US-11 — As an admin, I want to manage roles and see activity.**
- user listing, role change, global statistics
- 403 for a non-admin user, 401 for an anonymous one

**US-12 — As a low-vision user, I want to navigate the app with a screen reader.**
- `Semantics` labels, touch targets ≥ 48 dp, AA-compliant contrast
- detailed policy: [`accessibility/politique-accessibilite.md`](accessibility/politique-accessibilite.md)

### 4.5 GDPR compliance

**US-13 — As a user, I want to retrieve my personal data.**
- complete JSON export, **without** the password hash — the field's absence is structural, not a runtime filter

**US-14 — As a user, I want to delete my account.**
- 204 and cascading erasure (playlists, streams, tracks)
- a second deletion still succeeds: a network retry must not produce a server error
- the id comes **from the token**, never from the URL: acting on someone else's account is impossible by construction

### 4.6 Operations

**US-15 — As an operator, I want to tell a technical failure from a business problem.**
- separate metrics: 5xx errors on one side, login failures and disconnections on the other
- distinct dashboard panels

**US-16 — As an operator, I want to follow a request end to end.**
- a continuous trace from the mobile app down to the database
- every log line emitted inside a request carries its `trace_id`

**US-17 — As an operator, I want to know which version is running.**
- `/health` returns the deployed binary's version and commit

## 5. Non-functional requirements

### 5.1 Performance — measured, not estimated

| Requirement | Target | Measured |
|---|---|---|
| Simultaneous listeners on one stream | "N" (unquantified in the brief) | **1000**, no loss |
| Latency to first audio byte | < 1 s | **p99 = 117 ms** |
| Memory under load | "minimal" | **38.5 MiB** for 1000 listeners, 12 KiB/listener |
| Resource leaks | none | **goroutine delta +0** after disconnect |

Details and accepted limits: [`TEST_DE_CHARGE.en.md`](TEST_DE_CHARGE.en.md).

### 5.2 Security schema

| Layer | Measure |
|---|---|
| Transport | HTTPS enforced in production, HSTS outside development |
| Authentication | signed JWT, configurable expiry; bcrypt passwords |
| Authorisation | id taken from the token and never from the URL on personal routes; ownership checked in use cases, not in transport |
| Injection | parameterised queries only, no SQL concatenation |
| Abuse | per-IP rate limiting on authentication routes |
| Sensitive endpoints | `/metrics` behind a constant-time compared token; the API refuses to start in production without it |
| Browser | exact-allowlist CORS, `nosniff`, `X-Frame-Options: DENY`, `default-src 'none'` CSP |
| Supply chain | gitleaks, trivy and govulncheck blocking on every PR |

Rationale and trade-offs: [ADR 0012](adr/0012-hardening-avant-mise-en-production.en.md).

### 5.3 Quality

- Test coverage **≥ 80%**, enforced by CI (currently 82.6%)
- `go vet`, `staticcheck`, `gosec` warning-free; `flutter analyze` issue-free
- Race detection (`-race`) across the whole suite
- Detailed strategy: [`PLAN_DE_TESTS.en.md`](PLAN_DE_TESTS.en.md)

### 5.4 Operability

- **JSON** logs on standard output, correlated with traces
- Prometheus metrics, OpenTelemetry traces, Grafana dashboards
- Alerts on error rate and latency, each with its FR/EN runbook
- Configuration **exclusively through environment variables** (12-Factor): no hardcoded value

### 5.5 Accessibility

The application meets AA criteria (contrast, touch targets, screen reader). The **documentation** does too: heading structure, tables rather than diagrams alone, and every diagram accompanied by prose stating its content — an undescribed diagram is inaccessible to a screen reader.

## 6. Architecture

**Go backend** in Clean Architecture: `domain` (no dependencies) ← `application` ← `infrastructure` / `transport`. The dependency rule is one-way and visible on the [component diagram](diagrams/README.en.md#1-layered-architecture-component-diagram).

**Flutter mobile** organised by feature, BLoC state management, each feature carrying `bloc/`, `models/`, `repositories/`, `screens/`.

**Broadcasting**: one in-memory pub/sub hub per stream, fed by a goroutine and distributing over buffered channels, with `context.Context` for cancellation. That is what produces the non-starvation and leak-freedom measured in §5.1.

Structural decisions and the alternatives rejected are in [`adr/`](adr/).

## 7. Data model

Five tables: `users`, `playlists`, `playlist_tracks`, `streams`, `tracks`. Every foreign key to `users` is `ON DELETE CASCADE` — that is what makes GDPR erasure correct without the deletion code knowing these tables exist.

Entity-relationship diagram and specifics (notably the deferred position constraint used for reordering): [data model diagram](diagrams/README.en.md#2-data-model-entity-relationship).

## 8. Constraints

| Constraint | Origin | Handling |
|---|---|---|
| Go for the backend, Flutter for mobile | brief | respected |
| Clean Architecture / DDD | brief | dependency rule verifiable on the diagram |
| Unit testability ≥ 80% | brief | blocking CI gate |
| 12-Factor, zero hardcoding | brief | all configuration through environment variables |
| Multi-stage containerisation | brief | `alpine` image, non-root user, healthcheck |
| GDPR | regulatory | export, cascading erasure, [processing register](rgpd/registre-traitements.md) |
| Signed commits | team convention | "Verified" badge across the history |
| Zero-cost hosting | project constraint | free plan, with its limits documented in the runbook |

## 9. Organisation

Split into **vertical slices**: each member delivers a complete feature — backend, mobile, tests, instrumentation, ADR — rather than a horizontal layer. Each can therefore defend code they personally wrote, which is what Block 3's individual assessment requires.

| Member | Slice |
|---|---|
| 🟦 Yassir | Identity and trust: authentication, GDPR, observability, security, delivery, chat |
| 🟧 KaysZ | Streaming and media: broadcast hub, mobile player, upload |
| 🟪 SamyZ | Content and platform: playlists, administration, accessibility, security scans |

Rules: one PR per ticket, cross-review, never a self-merge, green CI before merging. Details in [`team/plan.md`](team/plan.md).

## 10. Deliverables

| Deliverable | Where |
|---|---|
| Source code | this repository |
| FR/EN technical documentation | [`docs/`](.) |
| Architecture decisions | [`docs/adr/`](adr/) |
| Standardised diagrams | [`docs/diagrams/`](diagrams/) |
| Test plan and acceptance book | [`PLAN_DE_TESTS.en.md`](PLAN_DE_TESTS.en.md), [`CAHIER_DE_RECETTE.en.md`](CAHIER_DE_RECETTE.en.md) |
| User training plan | [`user-guide/`](user-guide/) |
| Operations runbooks | [`docs/runbooks/`](runbooks/) |
| Deployed application | see the production deployment runbook |

## 11. Accepted limits and next steps

What the project **does not claim** to do, and why:

- **A single instance.** The broadcast hub is in memory; so is the rate limiter. Moving to N instances requires a message bus and a shared limiter. Both are written in the code, at the exact point where the constraint applies.
- **Measurements on one machine.** The load test runs client and server on the same host: the figures are a ceiling for the server side, not a production prediction.
- **The "N concurrent streams" profile is unmeasured.** The brief asks for N listeners on one stream; that is what is proven. The cost of N streams is dominated by other factors and remains to be measured.
- **Instrumentation stops at the HTTP layer.** Pushing traces down to SQL queries is the logical next step.
