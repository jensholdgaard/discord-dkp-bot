package telemetry

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"go.opentelemetry.io/contrib/bridges/otelslog"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploghttp"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	sdklog "go.opentelemetry.io/otel/sdk/log"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
	"go.opentelemetry.io/otel/trace"

	"github.com/jensholdgaard/discord-dkp-bot/internal/config"
)

// Provider holds all telemetry providers for shutdown.
type Provider struct {
	TracerProvider *sdktrace.TracerProvider
	MeterProvider  *sdkmetric.MeterProvider
	LoggerProvider *sdklog.LoggerProvider
	Logger         *slog.Logger
}

// Setup initializes OpenTelemetry traces, metrics and logs.
func Setup(ctx context.Context, cfg config.TelemetryConfig) (*Provider, error) {
	res, err := resource.New(ctx,
		resource.WithAttributes(
			semconv.ServiceNameKey.String(cfg.ServiceName),
			semconv.ServiceVersionKey.String(cfg.ServiceVersion),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("creating resource: %w", err)
	}

	traceOpts := []otlptracehttp.Option{otlptracehttp.WithEndpoint(cfg.OTLPEndpoint)}
	if cfg.Insecure {
		traceOpts = append(traceOpts, otlptracehttp.WithInsecure())
	}
	traceExp, err := otlptracehttp.New(ctx, traceOpts...)
	if err != nil {
		return nil, fmt.Errorf("creating trace exporter: %w", err)
	}
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(traceExp),
		sdktrace.WithResource(res),
	)
	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	metricOpts := []otlpmetrichttp.Option{otlpmetrichttp.WithEndpoint(cfg.OTLPEndpoint)}
	if cfg.Insecure {
		metricOpts = append(metricOpts, otlpmetrichttp.WithInsecure())
	}
	metricExp, err := otlpmetrichttp.New(ctx, metricOpts...)
	if err != nil {
		return nil, fmt.Errorf("creating metric exporter: %w", err)
	}
	mp := sdkmetric.NewMeterProvider(
		sdkmetric.WithReader(sdkmetric.NewPeriodicReader(metricExp)),
		sdkmetric.WithResource(res),
	)
	otel.SetMeterProvider(mp)

	logOpts := []otlploghttp.Option{otlploghttp.WithEndpoint(cfg.OTLPEndpoint)}
	if cfg.Insecure {
		logOpts = append(logOpts, otlploghttp.WithInsecure())
	}
	logExp, err := otlploghttp.New(ctx, logOpts...)
	if err != nil {
		return nil, fmt.Errorf("creating log exporter: %w", err)
	}
	lp := sdklog.NewLoggerProvider(
		sdklog.WithProcessor(sdklog.NewBatchProcessor(logExp)),
		sdklog.WithResource(res),
	)

	logger := otelslog.NewLogger(cfg.ServiceName, otelslog.WithLoggerProvider(lp))

	return &Provider{
		TracerProvider: tp,
		MeterProvider:  mp,
		LoggerProvider: lp,
		Logger:         logger,
	}, nil
}

// Shutdown gracefully shuts down all providers.
func (p *Provider) Shutdown(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	var errs []error
	if err := p.TracerProvider.Shutdown(ctx); err != nil {
		errs = append(errs, fmt.Errorf("tracer shutdown: %w", err))
	}
	if err := p.MeterProvider.Shutdown(ctx); err != nil {
		errs = append(errs, fmt.Errorf("meter shutdown: %w", err))
	}
	if err := p.LoggerProvider.Shutdown(ctx); err != nil {
		errs = append(errs, fmt.Errorf("logger shutdown: %w", err))
	}
	if len(errs) > 0 {
		return fmt.Errorf("shutdown errors: %v", errs)
	}
	return nil
}

// NewNopProvider returns a no-op provider for testing.
func NewNopProvider() *Provider {
	return &Provider{
		TracerProvider: sdktrace.NewTracerProvider(),
		MeterProvider:  sdkmetric.NewMeterProvider(),
		LoggerProvider: sdklog.NewLoggerProvider(),
		Logger:         slog.Default(),
	}
}

// LogWithTrace returns a logger enriched with trace_id and span_id from the context.
func LogWithTrace(ctx context.Context, logger *slog.Logger) *slog.Logger {
	sc := trace.SpanFromContext(ctx).SpanContext()
	if !sc.IsValid() {
		return logger
	}
	return logger.With(
		slog.String("trace_id", sc.TraceID().String()),
		slog.String("span_id", sc.SpanID().String()),
	)
}
