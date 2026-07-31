# StreamPulse

> Plateforme de streaming audio temps réel — Projet Semestriel 5A TL, S2 Bloc 3 (RNCP 38822) — École .decode.

StreamPulse permet à un *broadcaster* de diffuser un flux audio en direct vers N *listeners* simultanés, via une API Go et une application mobile Flutter. Construit par une équipe de 3, chacun responsable d'une tranche verticale complète (fonctionnalité + tests + CI/monitoring + documentation).

## Structure du projet

```
.
├── backend/          # API Go (bootstrap minimal : /health uniquement pour l'instant)
├── mobile/           # App Flutter (scaffold par défaut pour l'instant)
├── deployments/       # Docker, docker-compose, K8s, observabilité (à construire)
├── docs/
│   └── team/
│       ├── plan.md    # Répartition des tickets par membre
│       └── setup.md   # Setup git, signature GPG, workflow PR
└── .github/           # CODEOWNERS, templates PR/issue, CI
```

## Démarrage

```bash
cd backend && go build ./... && go test ./...
cd mobile && flutter pub get && flutter analyze && flutter test
```

## Équipe et répartition

Voir [`CONTRIBUTORS.md`](CONTRIBUTORS.md) et [`docs/team/plan.md`](docs/team/plan.md).

## Licence

Distribué sous licence MIT. Voir [`LICENSE`](LICENSE).
