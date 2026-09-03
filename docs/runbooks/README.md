# Runbooks — StreamPulse

> Périmètre actuel : les 5 alertes définies dans `deployments/prometheus/alerts.yml` — 3 sur l'authentification (ADR 0007), 2 sur la diffusion en direct. Le ticket S3 (SamyZ) étendra ce dossier avec l'alerting système complet (Postgres, Redis, disque...) et l'Alertmanager qui route réellement ces alertes vers un humain — aujourd'hui elles ne sont visibles que dans l'UI Prometheus (`/alerts`), pas encore notifiées.

## Index

| Alerte | Sévérité | Domaine | Runbook |
|---|---|---|---|
| `StreamPulseAuthHighFailureRate` | warning | métier | [alert-auth-high-failure-rate.md](alert-auth-high-failure-rate.md) |
| `StreamPulseAuthHighErrorRate` | critical | technique | [alert-auth-high-error-rate.md](alert-auth-high-error-rate.md) |
| `StreamPulseAuthHighLatency` | warning | technique | [alert-auth-high-latency.md](alert-auth-high-latency.md) |
| `StreamPulseListenersFallingBehind` | warning | métier | [alert-listeners-falling-behind.md](alert-listeners-falling-behind.md) |
| `StreamPulseBroadcastSilent` | warning | métier | [alert-broadcast-silent.md](alert-broadcast-silent.md) |

## Convention

Chaque runbook répond à 3 questions, dans cet ordre : **c'est quoi** (pour quelqu'un qui n'a pas écrit la règle), **comment vérifier que ce n'est pas un faux positif**, **quoi faire concrètement**. Pas de jargon non expliqué — un oncall qui découvre le projet doit pouvoir agir sans relire le code Go.
