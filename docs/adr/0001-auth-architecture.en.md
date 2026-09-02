# 0001 — Authentication architecture (ticket Y1)

## Status
Accepted

## Context
Three choices had to be made for the authentication feature: the JWT signing algorithm, the backend HTTP router, and the mobile state management approach.

## Decisions

### JWT with HS256 (not RS256)
StreamPulse is a single API consumed by a single mobile app — there is no multi-service scenario where a third party would need to verify a token without holding the shared secret. RS256 (public/private key pair) adds complexity (key management, rotation) with no benefit here. HS256 with a single shared secret (`JWT_SECRET`, ≥32 characters, never hardcoded) is enough.

### HTTP router: stdlib `net/http` (no framework)
Go 1.22+ added method+pattern routing (`mux.HandleFunc("POST /api/v1/auth/register", ...)`) directly to `net/http`. For the size of this API, adding Gin/Chi/Echo brings nothing we don't already have, and increases the dependency surface (which means attack surface and maintenance burden). Worth revisiting if the number of routes/middlewares grows significantly.

### Persistence: `database/sql` + `pgx` driver (no ORM)
Plain, explicit SQL, with no extra abstraction layer to learn or maintain. The query count stays low for this project's scope.

### Mobile: `flutter_bloc`
A clean event → state separation, unit-testable without any widget (see `test/features/auth/bloc/auth_bloc_test.dart`, 5 tests with `bloc_test`/`mocktail`, no UI rendering required). This choice should be followed by the other mobile tickets (K1, S1...) for consistency across the app.

## Consequences
- The JWT secret must be distributed securely in production (environment variable injected by the hosting platform, never committed).
- `internal/infrastructure/persistence` has no unit tests (it needs a real Postgres instance) — to be covered with testcontainers in a dedicated ticket if time allows.
