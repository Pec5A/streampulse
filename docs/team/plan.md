# StreamPulse — Plan de répartition équipe (v2, reconstruction à 3)

> **Référence** : Cahier des charges Decode "Projet Semestriel 5A TL S2 Bloc 3 — StreamPulse" (RNCP 38822 Bloc 3).
> **Équipe** : Yassir Sabbar (@JASSBR), KaysZ (@monkeyDkz), SamyZ (@SamyNikaia).
> **Inspiration fonctionnelle** : [Pec5A/streampulse-reference](https://github.com/Pec5A/streampulse-reference) — une implémentation antérieure qui marche déjà (backend Go + app Flutter), utilisée ici comme **référence/spec**, pas comme code à copier-coller. Chacun réécrit sa tranche depuis le cahier des charges ; le repo de référence sert à vérifier une approche, débloquer un choix d'archi, ou comparer un résultat — jamais à copier des fichiers tels quels.

---

## Pourquoi ce plan (rappel)

L'évaluation RNCP Bloc 3 se fait en deux temps :
1. **Présentation collective 10 min** (non notée).
2. **Entretien individuel 20 min par candidat** (noté) — le jury vérifie chaque critère du bloc.
3. **Règle stricte** : un seul "non acquis" → bloc invalidé pour ce candidat.

→ Chacun doit pouvoir défendre **du code qu'il a personnellement écrit**, sur les 6 axes A3.1→A3.6, **et** avoir construit de vraies fonctionnalités (pas seulement testé/durci l'existant). C'est précisément pour ça qu'on repart à 3 depuis zéro plutôt que de continuer sur le code solo de KaysZ.

---

## Principe : tickets verticaux, pas horizontaux

Contrairement à un découpage "phase tests / phase CI / phase docs", chaque ticket ci-dessous est une **tranche verticale complète** : la personne construit sa fonctionnalité (backend et/ou mobile), écrit ses propres tests, branche le monitoring pertinent, et documente son choix (ADR). Ça garantit que chacun a une histoire cohérente à raconter au jury sur "ce que j'ai construit", pas une liste de tâches éparpillées.

---

## Mapping critères RNCP par membre

| Membre | Slice fonctionnelle | A3.1 | A3.2 | A3.3 | A3.4 | A3.5 | A3.6 |
|---|---|---|---|---|---|---|---|
| 🟦 Yassir | Identité & confiance (auth, RGPD, chat bonus) | Pipeline CI initial | Tests auth | Dashboard auth | — | — | ADR auth |
| 🟧 KaysZ | Streaming & média (hub, player, upload) | — | Tests streaming | Dashboard métier/technique | CD (build+push+mobile) | Déploiement (compose/k8s) | ADR streaming |
| 🟪 SamyZ | Contenu & plateforme (playlists, admin, a11y) | Branch protection/commitlint | Tests playlists/admin | Alertes Prometheus + runbook | — | SLO/coûts | RGPD register + a11y |

---

## Phase 0 — Bootstrap (fait)

Squelette du repo : `backend/` (module Go + `cmd/api` minimal avec `/health`, testé), `mobile/` (scaffold Flutter par défaut, `flutter analyze`/`test` propres), `.github/` (CODEOWNERS, templates), `docs/team/`. Aucune fonctionnalité métier — juste de quoi que les 3 puissent démarrer en parallèle sans se marcher dessus.

---

## Phase 1 — Slices fonctionnelles (backend + mobile + tests, par personne)

### 🟦 Yassir — Identité & confiance

| # | Titre | Contenu | Critère |
|---|---|---|---|
| Y1 | Auth complet | Backend : entité `User`, JWT (register/login/refresh), hash bcrypt. Mobile : écrans login/register + stockage sécurisé du token. Tests des deux côtés. | A3.2, 15pts |
| Y2 | RGPD | Backend : export des données personnelles + suppression de compte (cascade). Tests. | A3.1, 15pts |

### 🟧 KaysZ — Streaming & média

| # | Titre | Contenu | Critère |
|---|---|---|---|
| K1 | Diffusion live | Backend : entité `Stream`, hub pub/sub (goroutines/channels) prouvant N auditeurs simultanés, endpoints publish (HTTP chunké + WebSocket navigateur) et listen. Mobile : lecteur audio complet (play/pause/volume/seek, lecture arrière-plan via `audio_service`, gestion des interruptions via `audio_session`). Tests des deux côtés. | A3.2, 15pts |
| K2 | Diffuseur & upload | Backend : upload de fichiers audio (stockage local, interface prête pour S3). Mobile : interface diffuseur (start/stop stream, upload track). Tests. | 15pts |

### 🟪 SamyZ — Contenu & plateforme

| # | Titre | Contenu | Critère |
|---|---|---|---|
| S1 | Playlists | Backend : CRUD playlists + tracks avec file d'attente/réordonnancement. Mobile : écran playlists + réordonnancement. Tests. | 15pts |
| S2 | Admin & accessibilité | Backend : gestion des rôles, liste utilisateurs, stats globales. Mobile : écran admin + passe d'accessibilité sur l'app entière (Semantics, contrastes, navigation clavier) + layout responsive. Tests. | A3.6, 15pts |

---

## Phase 2 — Infra transverse (une brique par personne)

| # | Titre | Owner | Critère |
|---|---|---|---|
| Y3 | Pipeline CI (lint, vet, gosec, coverage gate, Dart analyze) + dashboard Grafana auth (tentatives de connexion, business) | 🟦 Yassir | A3.1 |
| K3 | Pipeline CD (image Docker multi-stage → GHCR, build mobile APK/IPA) + déploiement (docker-compose, K8s) | 🟧 KaysZ | A3.4 |
| S3 | Scan sécurité (gitleaks, trivy, gosec) + alertes Prometheus/Alertmanager + runbook FR/EN + convention branch protection | 🟪 SamyZ | A3.3, A3.5 |

---

## Phase 3 — Documentation (chacun documente sa slice)

| # | Titre | Owner |
|---|---|---|
| Y-doc | ADR auth + RGPD, plan de formation utilisateurs | 🟦 Yassir |
| K-doc | ADR streaming, user stories, UML | 🟧 KaysZ |
| S-doc | Registre RGPD, politique accessibilité, README EN | 🟪 SamyZ |

---

## Phase 4 — Bonus (si le temps le permet)

| # | Titre | Owner | Note |
|---|---|---|---|
| Y4 | Chat WebSocket en direct par stream | 🟦 Yassir | Déjà construit et testé une fois (voir `streampulse-reference` PR #54) — à réintégrer proprement dans ce repo via une vraie PR, pas un copier-coller silencieux : même code, mais commit/PR/tests refaits ici pour que l'historique de CE repo (celui donné au jury) reste honnête. |
| K4 | Recommandation simple + transcodage adaptatif | 🟧 KaysZ | |
| S4 | Mode offline (cache playlists) | 🟪 SamyZ | |

---

## Conventions

### Branches
```
feat/<membre>-<scope>   ex. feat/kaysz-streaming-hub
test/<membre>-<scope>
ci/<membre>-<scope>
docs/<membre>-<scope>
fix/<membre>-<scope>
```

### Commits
- Conventional commits : `feat:`, `fix:`, `test:`, `ci:`, `docs:`, `chore:`, `refactor:`
- **Commits signés obligatoires** — chacun avec SA PROPRE clé GPG générée sur SA machine (voir `docs/team/setup.md`). Ne jamais partager une clé privée.
- Titre ≤ 72 caractères.

### PRs
- 1 PR = 1 ticket (Y1, K1, S1...)
- Template `.github/PULL_REQUEST_TEMPLATE.md` rempli intégralement
- Review croisée : un membre ne merge jamais sa propre PR
- CI verte avant merge

### Ordre conseillé
Chacun peut démarrer Phase 1 immédiatement en parallèle (aucune dépendance entre Y1/K1/S1). Phase 2 peut commencer dès que sa propre Phase 1 est mergée. Phase 3/4 en continu ou en fin de parcours selon le temps restant.
