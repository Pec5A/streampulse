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
- **Observabilité** — métriques Prometheus métier et techniques, deux dashboards Grafana (authentification, direct), 5 alertes avec runbooks, traces distribuées OpenTelemetry et logs JSON corrélés.

## Observabilité

Les trois signaux, et surtout le lien entre eux : une métrique dit *qu'il y a* un
problème, une trace dit *où*, un log dit *quoi*.

```bash
JWT_SECRET=change-me-to-at-least-32-characters docker compose up -d --build
curl localhost:8080/health
```

| Service | URL | Rôle |
|---|---|---|
| API | http://localhost:8080 | l'application |
| Métriques | http://localhost:8080/metrics | exposition Prometheus |
| Prometheus | http://localhost:9090 | collecte et alertes |
| Grafana | http://localhost:3000 (`admin` / `admin`) | dashboards, datasources Prometheus + Tempo provisionnées |
| Tempo | http://localhost:3200 | stockage des traces |

Les logs sortent en JSON sur stdout, chaque ligne émise dans une requête portant
`trace_id` et `span_id` : le même `trace_id` ouvre la trace correspondante dans
Grafana. Les spans sont nommés d'après le *template* de route
(`GET /api/v1/playlists/{id}`), jamais l'URL brute — voir
[`docs/adr/0011-otel-tracing-and-json-logs.md`](docs/adr/0011-otel-tracing-and-json-logs.md).

Le tracing est **désactivé par défaut** hors compose : sans
`OTEL_EXPORTER_OTLP_ENDPOINT`, l'API démarre normalement avec un simple Postgres.

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
| [`docs/cahier-des-charges.md`](docs/cahier-des-charges.md) | **Cahier des charges** : périmètre, user stories, exigences, schéma de sécurité, limites assumées |
| [`docs/diagrams/`](docs/diagrams/README.md) | Diagrammes Mermaid : composants, modèle de données, séquences (diffusion, trace), états, déploiement |
| [`docs/PLAN_DE_TESTS.md`](docs/PLAN_DE_TESTS.md) | Stratégie de tests, couverture mesurée, manques identifiés |
| [`docs/CAHIER_DE_RECETTE.md`](docs/CAHIER_DE_RECETTE.md) | Attentes fonctionnelles par rôle, et le test qui vérifie chacune |
| [`docs/TEST_DE_CHARGE.md`](docs/TEST_DE_CHARGE.md) | Mesures de charge : 1000 auditeurs sur un flux, 100 flux simultanés, coût CPU |
| [`docs/adr/`](docs/adr/) | Décisions d'architecture et leurs alternatives écartées (FR/EN) |
| [`docs/runbooks/`](docs/runbooks/) | Réponse aux incidents et procédure de mise en production (FR/EN) |
| [`docs/rgpd/`](docs/rgpd/), [`docs/accessibility/`](docs/accessibility/) | Registre des traitements, politique d'accessibilité |

## Équipe et workflow
SamyZ ([@SamyNikaia](https://github.com/SamyNikaia)), Yassir ([@JASSBR](https://github.com/JASSBR)), KaysZ ([@monkeyDkz](https://github.com/monkeyDkz)).
**Commits signés obligatoires** (badge « Verified »). Une PR par ticket, review croisée, jamais de self-merge. Voir [`docs/team/`](docs/team/) et [`docs/team/branch-protection.md`](docs/team/branch-protection.md).

## Licence
Distribué sous licence MIT. Voir [`LICENSE`](LICENSE).
