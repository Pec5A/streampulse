# Runbooks — StreamPulse

> Périmètre actuel : les 3 alertes auth définies dans `deployments/prometheus/alerts.yml` (ADR 0004). Le ticket S3 (SamyZ) étendra ce dossier avec l'alerting système complet (Postgres, Redis, disque...) et l'Alertmanager qui route réellement ces alertes vers un humain — aujourd'hui elles ne sont visibles que dans l'UI Prometheus (`/alerts`), pas encore notifiées.

## Index

| Alerte | Sévérité | Domaine | Runbook |
|---|---|---|---|
| `StreamPulseAuthHighFailureRate` | warning | métier | [alert-auth-high-failure-rate.md](alert-auth-high-failure-rate.md) |
| `StreamPulseAuthHighErrorRate` | critical | technique | [alert-auth-high-error-rate.md](alert-auth-high-error-rate.md) |
| `StreamPulseAuthHighLatency` | warning | technique | [alert-auth-high-latency.md](alert-auth-high-latency.md) |

## Convention

Chaque runbook répond à 3 questions, dans cet ordre : **c'est quoi** (pour quelqu'un qui n'a pas écrit la règle), **comment vérifier que ce n'est pas un faux positif**, **quoi faire concrètement**. Pas de jargon non expliqué — un oncall qui découvre le projet doit pouvoir agir sans relire le code Go.
