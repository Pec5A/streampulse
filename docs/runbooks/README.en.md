# Runbooks — StreamPulse

> Current scope: the 3 auth alerts defined in `deployments/prometheus/alerts.yml` (ADR 0004). Ticket S3 (SamyZ) will extend this folder with full system-wide alerting (Postgres, Redis, disk...) and the Alertmanager that actually routes these alerts to a human — today they're only visible in the Prometheus UI (`/alerts`), not yet notified.

## Index

| Alert | Severity | Domain | Runbook |
|---|---|---|---|
| `StreamPulseAuthHighFailureRate` | warning | business | [alert-auth-high-failure-rate.en.md](alert-auth-high-failure-rate.en.md) |
| `StreamPulseAuthHighErrorRate` | critical | technical | [alert-auth-high-error-rate.en.md](alert-auth-high-error-rate.en.md) |
| `StreamPulseAuthHighLatency` | warning | technical | [alert-auth-high-latency.en.md](alert-auth-high-latency.en.md) |

## Convention

Each runbook answers 3 questions, in this order: **what it is** (for someone who didn't write the rule), **how to rule out a false positive**, **what to actually do**. No unexplained jargon — an on-call engineer new to the project should be able to act without re-reading the Go code.
