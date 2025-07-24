package observability

import (
	"context"
	"errors"
	"time"

	"go.opentelemetry.io/otel/sdk/resource"
	semconv "go.opentelemetry.io/otel/semconv/v1.21.0"
)

const (
	otelAgentName        = "unified-ndp-agent"
	otelExporterEndpoint = "otel-collector:4317"
	otelMetricInterval   = 15 * time.Second
)

func InitProviders(ctx context.Context, nodeID string) (shutdown func(context.Context) error, err error) {
	res, err := resource.New(ctx, resource.WithAttributes(
		semconv.ServiceName(otelAgentName),
		semconv.HostIDKey.String(nodeID), // <-- ADD THIS LINE to tag all telemetry
	))
	if err != nil {
		return nil, err
	}

	meterProvider, err := initMeterProvider(ctx, res)
	if err != nil {
		return nil, err
	}
	if err := registerOtelMetrics(meterProvider); err != nil {
		return nil, err
	}


	shutdown = func(ctx context.Context) error {
		var errs error
		if err := meterProvider.Shutdown(ctx); err != nil {
			errs = errors.Join(errs, err)
		}
		return errs
	}

	return shutdown, nil
}