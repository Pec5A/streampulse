# Plan de tests itératif — StreamPulse

> Version anglaise : [`PLAN_DE_TESTS.en.md`](PLAN_DE_TESTS.en.md).
> Scénarios de recette : [`CAHIER_DE_RECETTE.md`](CAHIER_DE_RECETTE.md).
> Critères RNCP visés : **Ce3.2.1** à **Ce3.2.4**.

Ce document décrit comment le projet est testé, **et où il ne l'est pas encore**. Un plan de tests qui ne liste que ce qui va bien ne sert à rien : il n'aide ni à décider quoi écrire ensuite, ni à savoir quelle confiance accorder à un merge.

---

## 1. Principe : le test est écrit dans la même PR que le code

Le découpage de l'équipe est en **tranches verticales** (`docs/team/plan.md`) : une personne livre une fonctionnalité *avec* ses tests, son instrumentation et son ADR, dans une seule PR. Il n'y a donc pas de « phase de tests » à la fin — c'est ce que demande Ce3.2.2 (planification en parallèle du développement).

Conséquence concrète : **une PR sans test sur le code qu'elle ajoute n'est pas relue.** La CI ne peut pas mesurer ça toute seule, c'est la review croisée qui le tient.

## 2. Les cinq niveaux, et ce que chacun attrape

| Niveau | Où | Ce qu'il attrape | Ce qu'il ne peut pas attraper |
|---|---|---|---|
| **Unitaire (usecase)** | `internal/application/usecase/*_test.go` | règles métier, autorisations, cas d'erreur | tout ce qui touche au vrai HTTP ou à la vraie base |
| **Handler** | `internal/transport/http/handler/*_test.go` | codes de statut, forme du JSON, validation d'entrée | le routage : un handler correct branché sur la mauvaise route passe |
| **Routeur / bout-en-bout** | `internal/transport/http/router/*_test.go` | routage, middlewares, chaîne complète sur un `httptest.Server` | la persistance réelle (repositories en mémoire) |
| **Intégration** | `internal/infrastructure/persistence/*_integration_test.go`, tag `integration` | SQL réel, transactions, contraintes FK | l'UI |
| **Mobile** | `mobile/test/` | logique des BLoC, états émis, un test d'écran | le rendu réel sur device |

**Pourquoi ce découpage plutôt que « tout en unitaire »** : chaque niveau a déjà trouvé un bug qu'aucun autre ne pouvait voir. Le handler `publish` de K1 répondait `200 OK` *avant* de lire le corps — un client Go conforme arrête alors d'émettre, la diffusion était coupée au premier chunk. Aucun test à base de `httptest.ResponseRecorder` ne pouvait le montrer ; c'est le test bout-en-bout à 25 auditeurs qui l'a sorti.

## 3. État réel, mesuré

Chiffres relevés sur `main` (commit `930f99f`, 02/09/2026), reproductibles avec les commandes du §6.

| Package | Couverture | Tests |
|---|---|---|
| `internal/infrastructure/config` | **100 %** | 4 |
| `internal/transport/http/middleware` | **100 %** | 5 |
| `internal/transport/http/router` | **100 %** | 4 |
| `internal/infrastructure/auth` | **90,5 %** | 8 |
| `internal/transport/http/handler` | **89,3 %** | 23 |
| `internal/application/usecase` | **85,3 %** | 32 |
| `internal/infrastructure/persistence` | **0 %** | 1 (tag `integration`, **jamais exécuté en CI**) |
| `internal/application/dto`, `cmd/api` | 0 % | — (structures et câblage, sans logique) |
| **Total** | **58,1 %** | 77 |
| Mobile (`mobile/test/`) | — | 16, tous verts |

**Le total de 58,1 % n'est pas le chiffre à défendre, et il ne faut pas le maquiller.** Il est tiré vers le bas par `persistence` à 0 %, qui n'est pas du code non testé mais du code dont **les tests ne tournent pas**. Voir §4.

## 4. Les trois manques identifiés, par ordre de gravité

### 4.1 Les tests d'intégration ne tournent jamais — et ont pourri

`playlist_repository_integration_test.go` est derrière `//go:build integration`. Le job `Go Quality` lance `go test -race ./...` **sans le tag et sans base de données** : ce test n'a donc pas été exécuté une seule fois par la CI depuis son écriture.

Exécuté à la main contre un vrai Postgres le 02/09, il **échoue** :

```
playlist_repository_integration_test.go:43: seed user:
  ERROR: relation "users" does not exist (SQLSTATE 42P01)
```

Le test suppose une base déjà migrée au lieu d'établir lui-même son état. Il ne passait que si on le pointait par hasard sur une base qu'une API avait déjà migrée. C'est la démonstration exacte de Ce3.2.3 par la négative : **un test qui ne s'exécute pas automatiquement n'est pas un test, c'est un fichier.**

Aggravant : sur `main` il n'existe **aucun runner de migrations** — les fichiers SQL de `backend/migrations/` ne sont appliqués par rien. Le runner arrive avec la PR #14.

**Actions** : (a) faire appliquer les migrations par le test lui-même ; (b) ajouter un service Postgres au job `Go Quality` et lancer `-tags=integration` ; (c) mesurer avec `-coverpkg=./internal/...` et poser le seuil à 80 %.

### 4.2 Aucun seuil de couverture n'est appliqué

Le job `Go Quality` produit `coverage.out` et le publie en artefact — personne ne le lit. Une PR qui fait baisser la couverture passe au vert. L'exigence du sujet (« testable unitairement à 80 % minimum ») n'est donc vérifiée par rien.

**Action** : porte à 80 % sur le total mesuré avec `-coverpkg`, en échec bloquant.

### 4.3 Aucun test de charge, alors que c'est l'argument central du projet

Le sujet demande de « prouver que le serveur peut encaisser N auditeurs simultanés avec une consommation mémoire minimale ». La PR #22 a un test bout-en-bout à 25 auditeurs — c'est un test de correction, pas de charge : il ne mesure ni mémoire, ni latence, ni comportement à la saturation.

**Action** : un scénario k6 ou `vegeta` montant en auditeurs sur `/streams/{id}/listen`, avec relevé mémoire côté conteneur. À faire après le merge de #22.

## 5. Tests de sécurité (Ce3.2.1 « tests de sécurité »)

Automatisés, sur chaque PR, workflow `Security` :

| Outil | Ce qu'il couvre | Politique |
|---|---|---|
| **gitleaks** | secrets committés, historique complet | bloquant ; faux positif → `// gitleaks:allow` ou `.gitleaksignore` justifié |
| **trivy** | dépendances vulnérables (HIGH/CRITICAL, `--ignore-unfixed`) | bloquant |
| **govulncheck** | CVE Go *réellement atteignables* dans le graphe d'appels | bloquant |

Complétés par des tests d'autorisation écrits à la main, qui sont la vraie défense applicative : chaque route protégée a un test « sans token → 401 », « token d'un autre utilisateur → 403/404 », et les endpoints RGPD prennent l'identifiant **du JWT** et jamais un paramètre d'URL — ce qui rend l'IDOR impossible par construction plutôt que par vérification.

Réponse aux alertes : [`runbooks/security-incidents.md`](runbooks/security-incidents.md).

## 6. Reproduire les mesures

```bash
# Unitaire + couverture par package
cd backend && go test ./... -race -coverprofile=coverage.out
go tool cover -func=coverage.out | tail -1

# Intégration (nécessite un Postgres migré)
docker run -d --name pg -p 5432:5432 \
  -e POSTGRES_DB=streampulse -e POSTGRES_USER=streampulse -e POSTGRES_PASSWORD=streampulse \
  postgres:16-alpine
DATABASE_URL='postgres://streampulse:streampulse@localhost:5432/streampulse?sslmode=disable' \
  go test ./... -tags=integration -coverpkg=./internal/... -coverprofile=cov.out

# Mobile
cd mobile && flutter analyze && flutter test

# Détection de flakes : un test qui passe seul et tombe en suite complète
go test ./... -race -count=5
```

Le dernier n'est pas décoratif. `TestLiveStream_ChatFansOutToEveryParticipantIncludingTheSender` passait en isolation et tombait dans la suite complète sous `-race` : il écrivait son message dès le retour des `Dial`, alors que le handshake HTTP rendu ne garantit pas que la goroutine serveur a rejoint le salon. Corrigé en attendant le compte réel de participants.

## 7. Ce que la revue croisée vérifie, que la CI ne peut pas

- Le test **échoue-t-il** si on casse le code ? Un test qui ne peut pas rougir ne prouve rien.
- Le double (fake/mock) se comporte-t-il comme le vrai ? Sur la #14, `fakeUserRepo.Delete` était idempotent alors que le vrai renvoyait `ErrNotFound` : c'est cet écart qui masquait le bug, pas l'absence de test.
- Les cas d'erreur sont-ils testés autant que le chemin nominal ?
