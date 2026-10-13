package commands

import (
	"context"
	"os"
	"runtime"
	"runtime/debug"
	"time"

	"go.opentelemetry.io/contrib/exporters/autoexport"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/metric"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	semconv "go.opentelemetry.io/otel/semconv/v1.37.0"
)

// setupOTel configures metrics export. It is a no-op unless OTEL_METRICS_EXPORTER is set;
// exporter, endpoint, headers and interval come from the standard OTEL_* environment variables.
func setupOTel(ctx context.Context) (func(context.Context) error, error) {
	if os.Getenv("OTEL_METRICS_EXPORTER") == "" {
		return func(context.Context) error { return nil }, nil
	}

	version := "unknown"
	if info, ok := debug.ReadBuildInfo(); ok {
		version = info.Main.Version
	}

	res, err := resource.New(ctx,
		resource.WithAttributes(
			semconv.ServiceName("shinzo-gateway"),
			semconv.ServiceVersion(version),
			semconv.HostArchKey.String(runtime.GOARCH),
		),
		resource.WithOSType(),
		resource.WithTelemetrySDK(),
		resource.WithFromEnv(), // OTEL_RESOURCE_ATTRIBUTES can override
	)
	if err != nil {
		return nil, err
	}

	metricReader, err := autoexport.NewMetricReader(ctx)
	if err != nil {
		return nil, err
	}

	meterProvider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(metricReader), sdkmetric.WithResource(res))
	otel.SetMeterProvider(meterProvider)

	start := time.Now()
	_, err = meterProvider.Meter("github.com/shinzonetwork/shinzo-network-gateway/cmd/gateway/commands").Float64ObservableGauge("process.uptime",
		metric.WithDescription("Time since the process started."),
		metric.WithUnit("s"),
		metric.WithFloat64Callback(func(_ context.Context, o metric.Float64Observer) error {
			o.Observe(time.Since(start).Seconds())
			return nil
		}),
	)
	if err != nil {
		otel.Handle(err)
	}

	return meterProvider.Shutdown, nil
}
