# StreamPulse

> Plateforme de streaming audio temps réel — Projet Semestriel 5A TL, S2 Bloc 3 (RNCP 38822) — École .decode. English version: [`README.en.md`](README.en.md).

StreamPulse permet à un *broadcaster* de diffuser un flux audio en direct vers N *listeners* simultanés, via une API Go et une application mobile Flutter. Construit par une équipe de 3, chacun responsable d'une tranche verticale complète (fonctionnalité + tests + CI/monitoring + documentation).

## Architecture
- **Backend** (`backend/`) — Go, **Clean Architecture** (`domain` / `application` / `infrastructure` / `transport`), PostgreSQL via `database/sql` + pgx (sans ORM), routeur `net/http` standard, auth JWT. Décisions dans [`docs/adr/`](docs/adr/).
- **Mobile** (`mobile/`) — Flutter, gestion d'état `flutter_bloc`, structure par fonctionnalité (`lib/features/<feature>/{bloc,repository,screens,models}`).

## Fonctionnalités
Le statut est relatif à `main` ; les fonctionnalités encore en PR ouverte sont signalées.
- **Auth** — inscription / connexion / refresh (JWT, bcrypt). *Sur `main`.*
- **Admin** — gestion des rôles, liste des utilisateurs, statistiques (réservé admin). *Sur `main`.*
- **Scan de sécurité** — gitleaks, trivy, govulncheck en CI. *Sur `main`.*
- **Accessibilité** — libellés `Semantics`, cibles tactiles agrandies, layout responsive, contraste Material 3 (livré avec l'admin). *Sur `main`.*
- **Playlists** — CRUD + réordonnancement transactionnel des pistes, + cache offline. *En review (PR #16, #19).*
- **RGPD** — export des données personnelles & suppression de compte. *En review (PR #14).*
- **Streaming live & upload audio** — la fonctionnalité phare de la plateforme. *En cours dans des PR ouvertes (#22, #23) ; pas encore sur `main`.*

## Démarrage
```bash
# Backend (nécessite PostgreSQL)
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
GitHub Actions : qualité Go (vet, tests race, couverture), Flutter (analyze, test), et scan de sécurité (gitleaks + trivy + govulncheck).

## Documentation

| Document | Contenu |
|---|---|
| [`docs/diagrams/`](docs/diagrams/README.md) | Diagrammes Mermaid : composants, modèle de données, séquences (diffusion, trace), états, déploiement |
| [`docs/PLAN_DE_TESTS.md`](docs/PLAN_DE_TESTS.md) | Stratégie de tests, couverture mesurée, manques identifiés |
| [`docs/CAHIER_DE_RECETTE.md`](docs/CAHIER_DE_RECETTE.md) | Attentes fonctionnelles par rôle, et le test qui vérifie chacune |
| [`docs/adr/`](docs/adr/) | Décisions d'architecture et leurs limites assumées (FR/EN) |
| [`docs/runbooks/`](docs/runbooks/) | Réponse aux incidents et procédure de mise en production (FR/EN) |
| [`docs/rgpd/`](docs/rgpd/), [`docs/accessibility/`](docs/accessibility/) | Registre des traitements, politique d'accessibilité |

## Équipe et workflow
SamyZ ([@SamyNikaia](https://github.com/SamyNikaia)), Yassir ([@JASSBR](https://github.com/JASSBR)), KaysZ ([@monkeyDkz](https://github.com/monkeyDkz)).
**Commits signés obligatoires** (badge « Verified »). Une PR par ticket, review croisée, jamais de self-merge. Voir [`docs/team/`](docs/team/) et [`docs/team/branch-protection.md`](docs/team/branch-protection.md).

## Licence
Distribué sous licence MIT. Voir [`LICENSE`](LICENSE).
