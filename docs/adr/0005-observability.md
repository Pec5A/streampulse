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

## Extension — métriques de diffusion (septembre 2026)

La décision ci-dessus n'a longtemps couvert que l'authentification. Le direct — le cœur du produit — n'avait aucune métrique métier : on savait qu'une requête HTTP répondait, pas qu'un auditeur entendait quelque chose. Cinq métriques ont été ajoutées dans le groupe **métier**, dans `internal/infrastructure/observability/streaming_metrics.go` :

| Métrique | Type | Ce qu'elle répond |
|---|---|---|
| `streampulse_active_streams` | jauge | Combien de directs à l'antenne maintenant |
| `streampulse_active_listeners` | jauge | Combien de personnes écoutent maintenant |
| `streampulse_broadcast_bytes_total` | compteur | Volume audio publié depuis le démarrage |
| `streampulse_listener_chunks_dropped_total` | compteur | Auditeurs qui décrochent (signal avancé) |
| `streampulse_listener_evictions_total` | compteur | Auditeurs perdus pour cause de retard |

Trois choix structurent leur implémentation :

**Lecture au scrape, pas de compteur maintenu à la main.** Une jauge incrémentée à l'abonnement devrait être décrémentée sur les quatre façons de partir : désabonnement, éviction, fermeture du hub, arrêt du process. En rater une, c'est une jauge qui dérive en silence pour toute la vie du process. Le collecteur interroge le registre au moment du scrape (`Registry.Totals()`) : lire l'état vivant ne peut pas dériver. Bonus, `Hub.Publish` — le chemin chaud, une fois par chunk — n'est pas touché.

**Les compteurs ne redescendent jamais.** Quand un direct se termine, son hub meurt avec ses compteurs. Un total calculé seulement sur les hubs vivants chuterait, et Prometheus lit une chute comme un redémarrage de process, ce qui fausse tous les `rate()` traversant l'instant. Le registre absorbe donc les compteurs finaux de chaque hub à sa fermeture (`absorbLocked`). Attention au détail qui a coûté un bug : le retrait de la map, la fermeture du hub et l'absorption doivent tenir dans **une seule section critique**. Relâcher le verrou entre les deux ouvre une fenêtre — courte mais réelle, reproduite en test — où le hub n'est ni vivant ni retiré, et où un scrape lit un compteur qui recule. Couvert par `TestTotals_BytesNeverGoBackwardsWhileStreamsClose`.

**Aucune étiquette par flux.** Ce serait une série temporelle par diffusion jamais lancée : cardinalité non bornée, la façon classique de faire tomber un Prometheus. Le détail par flux vit dans les traces (ADR 0011), où les identifiants à forte cardinalité sont gratuits.

Conséquences :

- Second dashboard, `deployments/grafana/dashboards/streampulse-streaming.json` (« Direct (métier) »), 7 panneaux.
- Deux alertes métier de plus (`StreamPulseListenersFallingBehind`, `StreamPulseBroadcastSilent`) avec leurs runbooks — voir `docs/runbooks/`.
- `streampulse_broadcast_bytes_total` compte les octets **reçus des diffuseurs**, une fois par chunk quelle que soit l'audience. Ce n'est pas la bande passante sortante : avec N auditeurs, l'egress réel vaut environ N fois cette valeur. Les panneaux de débit le précisent explicitement.
- Le panneau de latence HTTP du dashboard auth a dû être filtré : `listen`, `publish` et `chat` sont des requêtes HTTP dont la durée est celle de la diffusion. Non filtrées, elles faisaient monter le p99 à une dizaine de secondes dès qu'un direct tournait, donnant l'impression d'une API en panne.
- La conséquence « `/metrics` exposé sans authentification » ci-dessus n'est plus d'actualité : l'endpoint est protégé par `middleware.RequireMetricsToken` (`METRICS_TOKEN`).
