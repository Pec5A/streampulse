# 0005 — Observabilité : séparer métier et technique (ticket Y3)

## Statut
Accepté

## Contexte
Le sujet demande explicitement de "différencier sur un dashboard Grafana les erreurs 500 (techniques) du nombre de déconnexions brutales d'utilisateurs (métier/expérience)". Il fallait décider comment structurer les métriques Prometheus pour que cette distinction soit réelle dans le code, pas juste dans la présentation du dashboard.

## Décision
Deux groupes de métriques, nommés et commentés comme tels dans `internal/infrastructure/observability/metrics.go` :

- **Métier** (`streampulse_auth_logins_total`, `streampulse_auth_registrations_total`) : incrémentées dans `AuthUseCase`, la couche qui connaît le *sens* métier d'un échec (mauvais mot de passe ≠ email déjà pris ≠ erreur serveur). Labellisées par résultat (`success`/`invalid_credentials`/`error`, `success`/`conflict`/`error`).
- **Technique** (`streampulse_http_requests_total`, `streampulse_http_request_duration_seconds`) : incrémentées par un middleware générique (`middleware.Metrics`) qui ne connaît que la route et le code HTTP — aucune logique métier.

Le dashboard (`deployments/grafana/dashboards/streampulse-auth.json`) a des panels séparés pour chaque groupe : connexions/inscriptions par résultat (métier) d'un côté, taux d'erreur 5xx et latence p50/p95/p99 (technique) de l'autre.

## Conséquences
- `/metrics` est exposé sans authentification pour l'instant (scraping local via docker-compose). À sécuriser si le service est un jour exposé publiquement (ticket S3).
- Le docker-compose (api + postgres + prometheus + grafana) a été testé avec un vrai `docker compose up` (voir ADR 0007) : ça a révélé que les migrations SQL ne s'appliquaient jamais automatiquement, corrigé dans le même lot de travail. Sans ce test réel, ce bug serait resté invisible jusqu'à la démo.
- Le fichier docker-compose sera probablement étendu par le ticket K3 (déploiement complet, K8s) — collaboration normale sur `deployments/`.
