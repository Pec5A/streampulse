# StreamPulse

> Real-time audio streaming platform — 5A TL semester project, S2 Bloc 3 (RNCP 38822), École .decode. Version française : [`README.md`](README.md).

StreamPulse lets a *broadcaster* stream live audio to many *listeners* at once, through a Go API and a Flutter mobile app. Built by a team of 3, each owning a full vertical slice (feature + tests + CI/monitoring + docs).

## Architecture
- **Backend** (`backend/`) — Go, **Clean Architecture** (`domain` / `application` / `infrastructure` / `transport`), PostgreSQL via `database/sql` + pgx (no ORM), standard-library `net/http` router, JWT auth. Decisions in [`docs/adr/`](docs/adr/).
- **Mobile** (`mobile/`) — Flutter, `flutter_bloc` state management, feature-first layout (`lib/features/<feature>/{bloc,repository,screens,models}`).

## Features
Status is relative to `main`; features still in open PRs are marked.
- **Auth** — register / login / refresh (JWT, bcrypt). *On `main`.*
- **Admin** — role management, user list, platform stats (admin-only). *On `main`.*
- **Security scanning** — gitleaks, trivy, govulncheck in CI. *On `main`.*
- **Accessibility** — Semantics labels, larger tap targets, responsive layout, Material 3 contrast (shipped with Admin). *On `main`.*
- **Playlists** — CRUD + transactional track reordering, plus an offline cache. *In review (PRs #16, #19).*
- **GDPR** — personal-data export & account deletion. *In review (PR #14).*
- **Live streaming & audio upload** — the platform's headline feature. *In progress in open PRs (#22, #23); not yet on `main`.*

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
GitHub Actions: Go quality (vet, race tests, coverage), Flutter (analyze, test), and security scanning (gitleaks + trivy + govulncheck).

## Documentation

| Document | Contents |
|---|---|
| [`docs/diagrams/`](docs/diagrams/README.en.md) | Mermaid diagrams: components, data model, sequences (streaming, tracing), states, deployment |
| [`docs/PLAN_DE_TESTS.en.md`](docs/PLAN_DE_TESTS.en.md) | Test strategy, measured coverage, identified gaps |
| [`docs/CAHIER_DE_RECETTE.en.md`](docs/CAHIER_DE_RECETTE.en.md) | Functional expectations per role, and the test verifying each |
| [`docs/adr/`](docs/adr/) | Architecture decisions and their accepted limits (FR/EN) |
| [`docs/runbooks/`](docs/runbooks/) | Incident response and production deployment procedure (FR/EN) |
| [`docs/rgpd/`](docs/rgpd/), [`docs/accessibility/`](docs/accessibility/) | Processing register, accessibility policy |

## Team & workflow
SamyZ ([@SamyNikaia](https://github.com/SamyNikaia)), Yassir ([@JASSBR](https://github.com/JASSBR)), KaysZ ([@monkeyDkz](https://github.com/monkeyDkz)).
**Signed commits required** ("Verified" badge). One PR per ticket, cross-review, no self-merge. See [`docs/team/`](docs/team/) and [`docs/team/branch-protection.md`](docs/team/branch-protection.md).

## License
MIT — see [`LICENSE`](LICENSE).
