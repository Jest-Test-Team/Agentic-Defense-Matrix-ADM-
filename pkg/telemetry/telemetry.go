package telemetry

import (
	"context"
	"fmt"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/metric"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
)

// Config holds telemetry configuration.
type Config struct {
	ServiceName    string
	ServiceVersion string
	OTLPEndpoint   string
	Environment    string
}

// Telemetry holds OTel providers and meters.
type Telemetry struct {
	Tracer trace.Tracer
	Meter  metric.Meter
	tp     *sdktrace.TracerProvider
}

// New initializes OTel with the given config.
func New(ctx context.Context, cfg Config) (*Telemetry, error) {
	// Tracer
	tp, err := sdktrace.NewTracerProvider(
		sdktrace.WithSampler(sdktrace.AlwaysSample()),
	)
	if err != nil {
		return nil, fmt.Errorf("create tracer provider: %w", err)
	}
	otel.SetTracerProvider(tp)

	tracer := tp.Tracer(cfg.ServiceName,
		trace.WithInstrumentationVersion(cfg.ServiceVersion),
	)

	// Meter
	meter := otel.Meter(cfg.ServiceName,
		metric.WithInstrumentationVersion(cfg.ServiceVersion),
	)

	return &Telemetry{
		Tracer: tracer,
		Meter:  meter,
		tp:     tp,
	}, nil
}

// Shutdown flushes pending telemetry.
func (t *Telemetry) Shutdown(ctx context.Context) error {
	return t.tp.Shutdown(ctx)
}

// Span starts a new span and returns it with a done function.
func (t *Telemetry) Span(ctx context.Context, name string) (context.Context, func()) {
	ctx, span := t.Tracer.Start(ctx, name)
	return ctx, func() { span.End() }
}

// SpanWithAttrs starts a span with attributes.
func (t *Telemetry) SpanWithAttrs(ctx context.Context, name string, attrs ...attribute.KeyValue) (context.Context, func()) {
	ctx, span := t.Tracer.Start(ctx, name, trace.WithAttributes(attrs...))
	return ctx, func() { span.End() }
}

// RecordError records an error on the current span.
func RecordError(ctx context.Context, err error) {
	span := trace.SpanFromContext(ctx)
	if span != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
	}
}

// AddEvent adds a span event with attributes.
func AddEvent(ctx context.Context, name string, attrs ...attribute.KeyValue) {
	span := trace.SpanFromContext(ctx)
	if span != nil {
		span.AddEvent(name, trace.WithAttributes(attrs...))
	}
}

// Histogram creates a histogram metric.
func (t *Telemetry) Histogram(ctx context.Context, name string, opts ...metric.Option) (metric.Float64Histogram, error) {
	return t.Meter.Float64Histogram(name, opts...)
}

// Counter creates a counter metric.
func (t *Telemetry) Counter(ctx context.Context, name string, opts ...metric.Option) (metric.Int64Counter, error) {
	return t.Meter.Int64Counter(name, opts...)
}

// Gauge creates an up-down counter metric.
func (t *Telemetry) Gauge(ctx context.Context, name string, opts ...metric.Option) (metric.Int64UpDownCounter, error) {
	return t.Meter.Int64UpDownCounter(name, opts...)
}

// Timer records a duration measurement.
func Timer(ctx context.Context, name string, start time.Time) {
	span := trace.SpanFromContext(ctx)
	if span != nil {
		span.SetAttributes(attribute.Float64(name+".ms", float64(time.Since(start).Microseconds())/1000.0))
	}
}
