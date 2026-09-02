package observability

import (
	"context"
	"fmt"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.43.0"
)

// TracingConfig is the subset of runtime configuration tracing needs. It is
// its own type rather than the whole *config.Config so that this package
// stays a leaf: infrastructure must not depend on the shape of the app's
// configuration struct.
type TracingConfig struct {
	ServiceName    string
	ServiceVersion string
	Environment    string
	// Endpoint is the OTLP gRPC collector address (host:port). Empty means
	// tracing is off — the normal case for unit tests and for anyone running
	// the API without the observability stack.
	Endpoint string
	// SampleRatio is the head sampling ratio, 0..1. 1 keeps every trace.
	SampleRatio float64
}

// InitTracing installs the global tracer provider and text-map propagator,
// and returns the function that flushes and shuts it down.
//
// The propagator is installed even when tracing is disabled. It is what reads
// and writes the W3C traceparent header, so installing it unconditionally
// means an incoming trace context is still carried through the process (and
// out to any downstream call) rather than being dropped on the floor the
// moment someone runs without a collector.
//
// The returned shutdown must be called on exit: the batch span processor holds
// finished spans in memory until its next export tick, so killing the process
// without it discards the spans of the last few seconds — which are precisely
// the ones an operator wants after an incident.
func InitTracing(ctx context.Context, cfg TracingConfig) (func(context.Context) error, error) {
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	// Validated before the early return: a bad ratio must fail now, not the
	// day someone plugs a collector in. Checking it after would let an
	// invalid configuration sit silently in an environment where tracing
	// happens to be off, and surface at the worst moment. Raised in review
	// by @monkeyDkz.
	if cfg.SampleRatio < 0 || cfg.SampleRatio > 1 {
		return nil, fmt.Errorf("trace sample ratio must be between 0 and 1, got %v", cfg.SampleRatio)
	}

	if cfg.Endpoint == "" {
		return func(context.Context) error { return nil }, nil
	}

	exporter, err := otlptracegrpc.New(ctx,
		otlptracegrpc.WithEndpoint(cfg.Endpoint),
		// The collector is a sidecar on the private network, not a public
		// endpoint; TLS is terminated at the ingress in front of it.
		otlptracegrpc.WithInsecure(),
	)
	if err != nil {
		return nil, fmt.Errorf("create otlp trace exporter: %w", err)
	}

	res, err := resource.Merge(
		resource.Default(),
		resource.NewWithAttributes(
			semconv.SchemaURL,
			semconv.ServiceName(cfg.ServiceName),
			semconv.ServiceVersion(cfg.ServiceVersion),
			attribute.String("deployment.environment.name", cfg.Environment),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("build otel resource: %w", err)
	}

	provider := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter, sdktrace.WithBatchTimeout(5*time.Second)),
		sdktrace.WithResource(res),
		// ParentBased so a sampling decision taken upstream is honoured:
		// without it a trace can be kept by the caller and dropped here,
		// leaving a hole in the middle of the very trace being followed.
		sdktrace.WithSampler(sdktrace.ParentBased(sdktrace.TraceIDRatioBased(cfg.SampleRatio))),
	)
	otel.SetTracerProvider(provider)

	return provider.Shutdown, nil
}
