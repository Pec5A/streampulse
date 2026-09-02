package observability

import (
	"context"
	"log/slog"
	"os"

	"go.opentelemetry.io/otel/trace"
)

// NewLogger builds the process logger.
//
// JSON, always — including in development. A log line is a record that a
// shipper (Loki, Elasticsearch) indexes field by field; the moment it is a
// sentence, extracting "which user", "which route", "which status" means
// writing and maintaining a regex per message shape. Keeping development on
// the same handler as production also means a log that parses in CI parses in
// prod, instead of the format only being exercised once deployed.
//
// Only the level differs per environment: debug locally, info elsewhere.
func NewLogger(environment string) *slog.Logger {
	level := slog.LevelInfo
	if environment == "development" {
		level = slog.LevelDebug
	}

	handler := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: level})
	return slog.New(&traceHandler{Handler: handler})
}

// traceHandler stamps every record that is emitted inside a span with its
// trace and span ids.
//
// This is what turns three separate signals into one investigation: a Grafana
// panel shows a latency spike (metric), the operator opens a trace from it,
// and the same trace_id pasted into Loki returns exactly the log lines that
// request produced. Without the correlation ids, logs and traces are two piles
// that can only be joined by guessing on timestamps.
type traceHandler struct {
	slog.Handler
}

func (h *traceHandler) Handle(ctx context.Context, record slog.Record) error {
	if sc := trace.SpanContextFromContext(ctx); sc.IsValid() {
		record.AddAttrs(
			slog.String("trace_id", sc.TraceID().String()),
			slog.String("span_id", sc.SpanID().String()),
		)
	}
	return h.Handler.Handle(ctx, record)
}

// WithAttrs and WithGroup must rewrap: the embedded handler returns a bare
// slog.Handler, so without these a logger.With(...) would silently lose the
// trace correlation from that point on.
//
// Caveat, asserted in logger_test.go: slog nests everything added during
// Handle under an open group, so a logger built with WithGroup emits
// group.trace_id instead of a root trace_id — and Grafana's log-to-trace link
// reads the root one. The API therefore never groups on the root logger.
func (h *traceHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &traceHandler{Handler: h.Handler.WithAttrs(attrs)}
}

func (h *traceHandler) WithGroup(name string) slog.Handler {
	return &traceHandler{Handler: h.Handler.WithGroup(name)}
}
