# StreamPulse

> English README. Version française : [`README.md`](README.md).

Audio streaming platform — semester team project (RNCP Bloc 3). Go backend + Flutter mobile app.

## Architecture
- **Backend** (`backend/`) — Go, **Clean Architecture** (`domain` / `application` / `infrastructure` / `transport`), PostgreSQL via `database/sql` + pgx (no ORM), standard-library `net/http` router, JWT auth. Architecture decisions live in [`docs/adr/`](docs/adr/).
- **Mobile** (`mobile/`) — Flutter, `flutter_bloc` state management, feature-first layout (`lib/features/<feature>/{bloc,repository,screens,models}`).

## Features
- **Auth** — register / login / refresh (JWT, bcrypt); GDPR data export & account deletion.
- **Playlists** — CRUD + track queue with **transactional reordering**; offline cache.
- **Admin** — role management, user list, platform stats (admin-only, gated).
- **Accessibility** — Semantics, 48dp targets, responsive layout, AA contrast.
- **Streaming & upload** — live audio broadcast and file upload (in progress).

## Run
```bash
# Backend (needs PostgreSQL)
cd backend
DATABASE_URL=postgres://user:pass@localhost:5432/streampulse?sslmode=disable \
  JWT_SECRET=change-me-to-at-least-32-characters go run ./cmd/api

# Mobile
cd mobile
flutter pub get
flutter run --dart-define=API_URL=http://localhost:8080
```

## Tests & CI
```bash
cd backend && go test -race -cover ./...
cd mobile && flutter analyze && flutter test
```
GitHub Actions runs: Go quality (vet, race tests, coverage), Flutter (analyze, test), and security scans (gitleaks, trivy, govulncheck).

## Team & workflow
SamyZ ([@SamyNikaia](https://github.com/SamyNikaia)), Yassir ([@JASSBR](https://github.com/JASSBR)), KaysZ ([@monkeyDkz](https://github.com/monkeyDkz)).
**Signed commits required** (SSH/GPG, "Verified" badge). One PR per ticket, cross-review, no self-merge. See [`docs/team/`](docs/team/) for the full workflow and [`docs/team/branch-protection.md`](docs/team/branch-protection.md) for the branch policy.
