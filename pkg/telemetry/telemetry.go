package telemetry

import (
	"context"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/metric"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
)

type Config struct {
	ServiceName    string
	ServiceVersion string
	OTLPEndpoint   string
	Environment    string
}

type Telemetry struct {
	Tracer trace.Tracer
	Meter  metric.Meter
	tp     *sdktrace.TracerProvider
}

func New(_ context.Context, cfg Config) (*Telemetry, error) {
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithSampler(sdktrace.AlwaysSample()),
	)
	otel.SetTracerProvider(tp)

	tracer := tp.Tracer(cfg.ServiceName,
		trace.WithInstrumentationVersion(cfg.ServiceVersion),
	)

	meter := otel.Meter(cfg.ServiceName,
		metric.WithInstrumentationVersion(cfg.ServiceVersion),
	)

	return &Telemetry{
		Tracer: tracer,
		Meter:  meter,
		tp:     tp,
	}, nil
}

func (t *Telemetry) Shutdown(ctx context.Context) error {
	return t.tp.Shutdown(ctx)
}

func (t *Telemetry) Span(ctx context.Context, name string) (context.Context, func()) {
	ctx, span := t.Tracer.Start(ctx, name)
	return ctx, func() { span.End() }
}

func (t *Telemetry) SpanWithAttrs(ctx context.Context, name string, attrs ...attribute.KeyValue) (context.Context, func()) {
	ctx, span := t.Tracer.Start(ctx, name, trace.WithAttributes(attrs...))
	return ctx, func() { span.End() }
}

func RecordError(ctx context.Context, err error) {
	span := trace.SpanFromContext(ctx)
	if span != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
	}
}

func AddEvent(ctx context.Context, name string, attrs ...attribute.KeyValue) {
	span := trace.SpanFromContext(ctx)
	if span != nil {
		span.AddEvent(name, trace.WithAttributes(attrs...))
	}
}

func Timer(ctx context.Context, name string, start time.Time) {
	span := trace.SpanFromContext(ctx)
	if span != nil {
		span.SetAttributes(attribute.Float64(name+".ms", float64(time.Since(start).Microseconds())/1000.0))
	}
}
