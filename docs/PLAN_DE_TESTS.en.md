# Iterative test plan — StreamPulse

> French version: [`PLAN_DE_TESTS.md`](PLAN_DE_TESTS.md).
> Acceptance scenarios: [`CAHIER_DE_RECETTE.en.md`](CAHIER_DE_RECETTE.en.md).
> RNCP criteria: **Ce3.2.1** to **Ce3.2.4**.

This document describes how the project is tested, **and where it is not yet**. A test plan that only lists what works is useless: it helps neither decide what to write next, nor judge how much to trust a merge.

---

## 1. Principle: the test ships in the same PR as the code

The team is split into **vertical slices** (`docs/team/plan.md`): one person delivers a feature *with* its tests, its instrumentation and its ADR, in a single PR. There is therefore no "testing phase" at the end — which is what Ce3.2.2 asks for (planning carried out in parallel with development).

In practice: **a PR with no test on the code it adds does not get reviewed.** CI cannot measure that on its own; cross-review is what holds it.

## 2. The five levels, and what each one catches

| Level | Where | What it catches | What it cannot catch |
|---|---|---|---|
| **Unit (use case)** | `internal/application/usecase/*_test.go` | business rules, authorisation, error paths | anything involving real HTTP or a real database |
| **Handler** | `internal/transport/http/handler/*_test.go` | status codes, JSON shape, input validation | routing: a correct handler wired to the wrong route still passes |
| **Router / end-to-end** | `internal/transport/http/router/*_test.go` | routing, middleware, full chain over an `httptest.Server` | real persistence (in-memory repositories) |
| **Integration** | `internal/infrastructure/persistence/*_integration_test.go`, `integration` tag | real SQL, transactions, FK constraints | the UI |
| **Mobile** | `mobile/test/` | BLoC logic, emitted states, one screen test | real rendering on a device |

**Why this split rather than "everything as unit tests"**: each level has already found a bug no other level could see. K1's `publish` handler answered `200 OK` *before* reading the body — a conforming Go client then stops sending, so the broadcast was cut at the first chunk. No `httptest.ResponseRecorder` test could show that; the 25-listener end-to-end test is what surfaced it.

## 3. Measured state

Figures taken on `main` (commit `930f99f`, 2026-09-02), reproducible with §6.

| Package | Coverage | Tests |
|---|---|---|
| `internal/infrastructure/config` | **100%** | 4 |
| `internal/transport/http/middleware` | **100%** | 5 |
| `internal/transport/http/router` | **100%** | 4 |
| `internal/infrastructure/auth` | **90.5%** | 8 |
| `internal/transport/http/handler` | **89.3%** | 23 |
| `internal/application/usecase` | **85.3%** | 32 |
| `internal/infrastructure/persistence` | **0%** | 1 (`integration` tag, **never run in CI**) |
| `internal/application/dto`, `cmd/api` | 0% | — (structs and wiring, no logic) |
| **Total** | **58.1%** | 77 |
| Mobile (`mobile/test/`) | — | 16, all green |

**The 58.1% total is not the number to defend, and must not be dressed up.** It is dragged down by `persistence` at 0%, which is not untested code but code **whose tests do not run**. See §4.

## 4. Three identified gaps, worst first

### 4.1 Integration tests never run — and have rotted

`playlist_repository_integration_test.go` sits behind `//go:build integration`. The `Go Quality` job runs `go test -race ./...` **without the tag and without a database**: that test has not been executed once by CI since it was written.

Run by hand against a real Postgres on 2026-09-02, it **fails**:

```
playlist_repository_integration_test.go:43: seed user:
  ERROR: relation "users" does not exist (SQLSTATE 42P01)
```

The test assumes an already-migrated database instead of establishing its own state. It only passed when pointed, by luck, at a database some API had already migrated. This is Ce3.2.3 demonstrated in the negative: **a test that does not run automatically is not a test, it is a file.**

Worse: `main` has **no migration runner at all** — nothing applies the SQL files in `backend/migrations/`. The runner arrives with PR #14.

**Actions**: (a) have the test apply migrations itself; (b) add a Postgres service to `Go Quality` and run `-tags=integration`; (c) measure with `-coverpkg=./internal/...` and set the gate at 80%.

### 4.2 No coverage threshold is enforced

`Go Quality` produces `coverage.out` and uploads it as an artefact — nobody reads it. A PR that lowers coverage goes green. The brief's requirement ("unit-testable to at least 80%") is therefore checked by nothing.

**Action**: an 80% gate on the total measured with `-coverpkg`, blocking.

### 4.3 No load test, although that is the project's central claim

The brief asks to "prove the server can absorb N simultaneous listeners with minimal memory consumption". PR #22 has a 25-listener end-to-end test — that is a correctness test, not a load test: it measures neither memory, nor latency, nor behaviour at saturation.

**Action**: a k6 or `vegeta` scenario ramping listeners on `/streams/{id}/listen`, with container memory readings. To be done once #22 is merged.

## 5. Security testing (Ce3.2.1 "security tests")

Automated, on every PR, `Security` workflow:

| Tool | Scope | Policy |
|---|---|---|
| **gitleaks** | committed secrets, full history | blocking; false positive → `// gitleaks:allow` or a justified `.gitleaksignore` entry |
| **trivy** | vulnerable dependencies (HIGH/CRITICAL, `--ignore-unfixed`) | blocking |
| **govulncheck** | Go CVEs *actually reachable* in the call graph | blocking |

Complemented by hand-written authorisation tests, which are the real application-level defence: every protected route has a "no token → 401" and "another user's token → 403/404" test, and the GDPR endpoints take the id **from the JWT**, never a URL parameter — which makes IDOR impossible by construction rather than by checking.

Alert response: [`runbooks/security-incidents.md`](runbooks/security-incidents.md).

## 6. Reproducing the measurements

```bash
# Unit + per-package coverage
cd backend && go test ./... -race -coverprofile=coverage.out
go tool cover -func=coverage.out | tail -1

# Integration (needs a migrated Postgres)
docker run -d --name pg -p 5432:5432 \
  -e POSTGRES_DB=streampulse -e POSTGRES_USER=streampulse -e POSTGRES_PASSWORD=streampulse \
  postgres:16-alpine
DATABASE_URL='postgres://streampulse:streampulse@localhost:5432/streampulse?sslmode=disable' \
  go test ./... -tags=integration -coverpkg=./internal/... -coverprofile=cov.out

# Mobile
cd mobile && flutter analyze && flutter test

# Flake detection: a test that passes alone and fails in the full suite
go test ./... -race -count=5
```

The last one is not decorative. `TestLiveStream_ChatFansOutToEveryParticipantIncludingTheSender` passed in isolation and failed in the full suite under `-race`: it wrote its message as soon as the `Dial` calls returned, while a completed HTTP handshake does not guarantee the server goroutine has joined the room. Fixed by waiting on the real participant count.

## 7. What cross-review checks that CI cannot

- Does the test **fail** when the code is broken? A test that cannot go red proves nothing.
- Does the double (fake/mock) behave like the real thing? On #14, `fakeUserRepo.Delete` was idempotent while the real one returned `ErrNotFound`: that divergence is what hid the bug, not a missing test.
- Are error paths tested as thoroughly as the happy path?
