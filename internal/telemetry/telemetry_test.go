package telemetry_test

import (
	"context"
	"log/slog"
	"testing"

	"go.opentelemetry.io/otel/trace"

	"github.com/jensholdgaard/discord-dkp-bot/internal/telemetry"
)

func TestNewNopProvider(t *testing.T) {
	p := telemetry.NewNopProvider()

	if p.TracerProvider == nil {
		t.Fatal("TracerProvider is nil")
	}
	if p.MeterProvider == nil {
		t.Fatal("MeterProvider is nil")
	}
	if p.LoggerProvider == nil {
		t.Fatal("LoggerProvider is nil")
	}
	if p.Logger == nil {
		t.Fatal("Logger is nil")
	}
}

func TestNopProvider_Shutdown(t *testing.T) {
	p := telemetry.NewNopProvider()
	if err := p.Shutdown(context.Background()); err != nil {
		t.Fatalf("Shutdown() error = %v", err)
	}
}

func TestLogWithTrace_NoSpan(t *testing.T) {
	logger := slog.Default()
	// Context with no span should return the same logger.
	got := telemetry.LogWithTrace(context.Background(), logger)
	if got == nil {
		t.Fatal("LogWithTrace() returned nil")
	}
}

func TestLogWithTrace_WithSpan(t *testing.T) {
	p := telemetry.NewNopProvider()
	tracer := p.TracerProvider.Tracer("test")
	ctx, span := tracer.Start(context.Background(), "test-span")
	defer span.End()

	logger := slog.Default()
	enriched := telemetry.LogWithTrace(ctx, logger)
	if enriched == nil {
		t.Fatal("LogWithTrace() returned nil")
	}

	// The enriched logger should differ from the original because
	// we added trace_id/span_id, but we can't easily check that
	// since the noop tracer produces an invalid span context.
	// At a minimum, verify it returns without panic.
	sc := trace.SpanFromContext(ctx).SpanContext()
	_ = sc // validates no panic
}
