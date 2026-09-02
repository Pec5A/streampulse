package observability

import (
	"context"
	"testing"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
)

func TestInitTracing_DisabledWithoutAnEndpoint(t *testing.T) {
	// The API has to stay runnable with nothing but a Postgres. No endpoint
	// must mean "no tracing", never "refuse to boot".
	shutdown, err := InitTracing(context.Background(), TracingConfig{
		ServiceName: "streampulse-api",
		SampleRatio: 1,
	})
	if err != nil {
		t.Fatalf("InitTracing() error = %v, want nil when tracing is off", err)
	}
	if shutdown == nil {
		t.Fatal("shutdown is nil; callers defer it unconditionally")
	}
	if err := shutdown(context.Background()); err != nil {
		t.Errorf("shutdown() error = %v, want nil", err)
	}
}

func TestInitTracing_InstallsPropagatorEvenWhenDisabled(t *testing.T) {
	// Without the propagator a traceparent arriving from the mobile app is
	// dropped, so a request that *is* traced upstream starts a fresh,
	// disconnected trace here the moment someone runs without a collector.
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator())

	if _, err := InitTracing(context.Background(), TracingConfig{SampleRatio: 1}); err != nil {
		t.Fatalf("InitTracing() error = %v", err)
	}

	fields := otel.GetTextMapPropagator().Fields()
	var hasTraceparent bool
	for _, f := range fields {
		if f == "traceparent" {
			hasTraceparent = true
		}
	}
	if !hasTraceparent {
		t.Errorf("propagator fields = %v, want traceparent (W3C trace context)", fields)
	}
}

func TestInitTracing_RejectsAnOutOfRangeSampleRatio(t *testing.T) {
	tests := []struct {
		name  string
		ratio float64
	}{
		{"negative", -0.1},
		{"above one", 1.5},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := InitTracing(context.Background(), TracingConfig{
				Endpoint:    "localhost:4317",
				SampleRatio: tc.ratio,
			})
			if err == nil {
				t.Fatalf("InitTracing(ratio=%v) error = nil, want a validation error", tc.ratio)
			}
		})
	}
}
