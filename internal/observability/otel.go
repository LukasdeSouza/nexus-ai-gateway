package observability

import (
	"context"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
	"go.opentelemetry.io/otel/trace"

	"github.com/LukasdeSouza/nexus-ai-gateway/internal/config"
)

// OTelSDK manages tracing lifecycle.
type OTelSDK struct {
	tp *sdktrace.TracerProvider
}

// Setup initializes OpenTelemetry tracing with OTLP HTTP exporter.
func Setup(ctx context.Context, cfg config.ObservabilityConfig) (*OTelSDK, error) {
	if !cfg.TracingEnabled {
		return &OTelSDK{}, nil
	}

	exporter, err := otlptracehttp.New(ctx,
		otlptracehttp.WithEndpointURL(cfg.OTLPEndpoint),
		otlptracehttp.WithInsecure(),
	)
	if err != nil {
		return nil, err
	}

	res, err := resource.New(ctx,
		resource.WithAttributes(
			semconv.ServiceName(cfg.ServiceName),
		),
	)
	if err != nil {
		return nil, err
	}

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(res),
	)

	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	return &OTelSDK{tp: tp}, nil
}

// Shutdown flushes and closes tracing providers.
func (s *OTelSDK) Shutdown(ctx context.Context) error {
	if s.tp != nil {
		return s.tp.Shutdown(ctx)
	}
	return nil
}

// Tracer returns an OpenTelemetry tracer for the component.
func Tracer(name string) trace.Tracer {
	return otel.GetTracerProvider().Tracer(name)
}
