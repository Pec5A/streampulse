package observability

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"testing"

	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

// newTestLogger mirrors NewLogger but writes to buf, so the assertions can
// read the bytes that would have gone to stdout.
func newTestLogger(buf *bytes.Buffer) *slog.Logger {
	h := slog.NewJSONHandler(buf, &slog.HandlerOptions{Level: slog.LevelDebug})
	return slog.New(&traceHandler{Handler: h})
}

func decode(t *testing.T, buf *bytes.Buffer) map[string]any {
	t.Helper()

	var got map[string]any
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatalf("log line is not valid JSON (%v): %s", err, buf.String())
	}
	return got
}

func TestNewLogger_EmitsJSON(t *testing.T) {
	var buf bytes.Buffer
	newTestLogger(&buf).Info("database connected", "attempts", 2)

	got := decode(t, &buf)
	if got["msg"] != "database connected" {
		t.Errorf("msg = %v, want %q", got["msg"], "database connected")
	}
	if got["attempts"] != float64(2) {
		t.Errorf("attempts = %v, want 2 as its own field, not folded into the message", got["attempts"])
	}
	if got["level"] != "INFO" {
		t.Errorf("level = %v, want INFO", got["level"])
	}
}

func TestNewLogger_AddsTraceCorrelationInsideASpan(t *testing.T) {
	provider := sdktrace.NewTracerProvider()
	t.Cleanup(func() { _ = provider.Shutdown(context.Background()) })

	ctx, span := provider.Tracer("test").Start(context.Background(), "op")
	defer span.End()

	var buf bytes.Buffer
	newTestLogger(&buf).InfoContext(ctx, "handling request")

	got := decode(t, &buf)
	wantTrace := span.SpanContext().TraceID().String()
	if got["trace_id"] != wantTrace {
		t.Errorf("trace_id = %v, want %s", got["trace_id"], wantTrace)
	}
	wantSpan := span.SpanContext().SpanID().String()
	if got["span_id"] != wantSpan {
		t.Errorf("span_id = %v, want %s", got["span_id"], wantSpan)
	}
}

func TestNewLogger_NoTraceFieldsOutsideASpan(t *testing.T) {
	var buf bytes.Buffer
	newTestLogger(&buf).InfoContext(context.Background(), "starting up")

	got := decode(t, &buf)
	if _, ok := got["trace_id"]; ok {
		t.Error("trace_id present with no active span; an empty id would poison log/trace joins")
	}
}

// Regression guard: slog.Handler.WithAttrs returns a plain slog.Handler, so a
// wrapper that forgets to rewrap silently drops its own behaviour on every
// logger built with .With(...) — which is most of them.
func TestNewLogger_KeepsCorrelationThroughWith(t *testing.T) {
	provider := sdktrace.NewTracerProvider()
	t.Cleanup(func() { _ = provider.Shutdown(context.Background()) })

	ctx, span := provider.Tracer("test").Start(context.Background(), "op")
	defer span.End()

	var buf bytes.Buffer
	newTestLogger(&buf).With("component", "auth").InfoContext(ctx, "denied")

	got := decode(t, &buf)
	if got["trace_id"] != span.SpanContext().TraceID().String() {
		t.Errorf("trace_id lost after With(): %v", got["trace_id"])
	}
	if got["component"] != "auth" {
		t.Errorf("component = %v, want auth", got["component"])
	}
}

// Documents a real constraint rather than leaving it to be discovered in
// production: slog nests every attribute added during Handle under whatever
// group is open, correlation ids included. Grafana's log-to-trace link reads a
// *root* trace_id, so a grouped logger would silently stop linking. The API
// therefore never opens a group on the root logger — asserted here so that
// changing that has to be a deliberate decision with a failing test, not an
// accident nobody notices until an incident.
func TestNewLogger_GroupNestsCorrelationIdsAwayFromTheRoot(t *testing.T) {
	provider := sdktrace.NewTracerProvider()
	t.Cleanup(func() { _ = provider.Shutdown(context.Background()) })

	ctx, span := provider.Tracer("test").Start(context.Background(), "op")
	defer span.End()

	var buf bytes.Buffer
	newTestLogger(&buf).WithGroup("http").InfoContext(ctx, "denied")

	got := decode(t, &buf)
	if _, ok := got["trace_id"]; ok {
		t.Error("trace_id is at the root under a group — behaviour changed, revisit the note in logger.go")
	}
	group, ok := got["http"].(map[string]any)
	if !ok {
		t.Fatalf("http group missing: %v", got)
	}
	if group["trace_id"] != span.SpanContext().TraceID().String() {
		t.Errorf("group trace_id = %v, want the span's", group["trace_id"])
	}
}

func TestNewLogger_LevelFollowsEnvironment(t *testing.T) {
	tests := []struct {
		environment string
		wantDebug   bool
	}{
		{"development", true},
		{"production", false},
		{"staging", false},
	}

	for _, tc := range tests {
		t.Run(tc.environment, func(t *testing.T) {
			logger := NewLogger(tc.environment)
			if got := logger.Enabled(context.Background(), slog.LevelDebug); got != tc.wantDebug {
				t.Errorf("debug enabled = %v, want %v", got, tc.wantDebug)
			}
			if !logger.Enabled(context.Background(), slog.LevelInfo) {
				t.Error("info must be enabled in every environment")
			}
		})
	}
}
