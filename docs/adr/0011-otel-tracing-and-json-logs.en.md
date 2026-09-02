# 0011 — OpenTelemetry traces and correlated JSON logs (ticket Y3, continued)

## Status
Accepted

## Context
Two explicit requirements of the brief were met by no branch in the repository:

- "Instrumenting the code to produce **distributed traces**. A student must be able to follow a request from the mobile application down to the database."
- "**Dropping plain text** in favour of structured logs allowing automated analysis (via Loki or Elasticsearch)."

ADR 0005 delivered Prometheus metrics and the dashboard. Two of the three observability signals were therefore missing — and, more importantly, the link between them: a metric says *that* something is wrong, a trace says *where*, a log says *what*. With no correlation you have three tools and no investigation.

## Decision

### 1. Traces: OTLP to a collector, never straight to a backend
`observability.InitTracing` installs a `TracerProvider` exporting over OTLP/gRPC to an **OpenTelemetry Collector**, which in turn exports to **Tempo**. The backend therefore knows one endpoint and one protocol; swapping Tempo for Jaeger, adding sampling, or fanning out to a second backend is a change to `deployments/otel-collector/config.yaml`, not a redeploy of the API.

Three choices are worth justifying:

- **The W3C propagator is installed even when tracing is disabled.** It is what reads the `traceparent` header. Without it, a request already traced upstream (the Flutter app) would arrive here as a fresh, unrelated trace — the trace would break precisely at the process that turned tracing off for convenience.
- **`ParentBased` sampling.** If the sampling decision was taken upstream, it is honoured. A ratio decided independently at each tier punches holes in the middle of the very trace being followed.
- **`shutdown` is mandatory, and gets its own context.** The batch span processor holds finished spans in memory until its next export tick; killing the process without flushing discards the last few seconds of spans — exactly the ones wanted after an incident. The flush context is not derived from the shutdown context, which is already cancelled by the time it would be used.

### 2. The span name is the **route template**, never the URL
`GET /api/v1/playlists/{id}`, not `/api/v1/playlists/3f2b…`. Same reason as the Prometheus label: a raw URL mints a distinct operation per id ever opened, making traces ungroupable and blowing up cardinality in the trace backend. Requests matching no route fall back to the constant `<unmatched>` label, for the same bounded-cardinality reason as in `middleware.Metrics`.

Only **5xx** marks a span as failed. A 401 or a 404 is the server working; counting those as errors would make the error rate track client behaviour instead of service health — the business/technical confusion ADR 0005 exists to avoid.

### 3. Logs: JSON everywhere, with `trace_id` at the root
`observability.NewLogger` returns an `slog` logger on a JSON handler, **including in development**: a format only exercised in production is a format discovered broken in production. Only the level differs (debug locally, info elsewhere).

The handler wraps the JSON handler to stamp every record emitted inside a span with `trace_id` and `span_id`. That is what joins the three signals: a latency spike on a Grafana panel → open the trace → the same `trace_id` pasted into Loki returns exactly the log lines that request produced.

## Consequences
- **Correlation only fires on the context-carrying variants.** `slog.Info(...)` has no `context.Context`, so no span, so no `trace_id` — the handler cannot invent one. Learned the hard way: the first full bring-up produced flawless traces and **zero correlated log lines**, because the streaming handler's three call sites used the context-free variants. The rule is therefore: inside a request path, always `InfoContext`/`ErrorContext`. A linter (`sloglint`, `context` rule) would make this structural rather than a matter of discipline; that is not in place yet.
- **Known and tested constraint**: `slog` nests every attribute added during `Handle` under whatever group is open, correlation ids included. A logger built with `WithGroup` would emit `http.trace_id` instead of a root `trace_id`, which Grafana's log-to-trace link would not find. The API therefore never groups on the root logger, and `TestNewLogger_GroupNestsCorrelationIdsAwayFromTheRoot` locks that behaviour in: changing it takes a deliberate decision with a failing test, not an accident nobody notices until the next incident.
- Tracing is **off by default** (`OTEL_EXPORTER_OTLP_ENDPOINT` empty). The API must stay bootable with nothing but a Postgres; making the collector mandatory would turn the observability stack into a prerequisite for every developer.
- An invalid sample ratio **fails startup** rather than silently degrading to 0: an apparently healthy service producing no traces is only discovered on the day one is needed.
- The `docker compose` stack gains two services (`otel-collector`, `tempo`) and a Grafana datasource. The Prometheus datasource uid is now pinned — without it Grafana generates a random one and Tempo's trace-to-metrics link stops resolving on the next `docker compose up`.
- Instrumentation stops at the HTTP layer. Pushing spans down to SQL queries (`otelpgx`) is the logical next step and is not done here.
