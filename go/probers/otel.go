package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	gcpmetric "github.com/GoogleCloudPlatform/opentelemetry-operations-go/exporter/metric"
	gcptrace "github.com/GoogleCloudPlatform/opentelemetry-operations-go/exporter/trace"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	otelcodes "go.opentelemetry.io/otel/codes"
	otelmetric "go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/sdk/metric"
	otelmetricdata "go.opentelemetry.io/otel/sdk/metric/metricdata"
	"go.opentelemetry.io/otel/sdk/resource"
	"go.opentelemetry.io/otel/sdk/trace"
	oteltrace "go.opentelemetry.io/otel/trace"
	"google.golang.org/api/option"
)

type otelRuntime struct {
	shutdown         func(context.Context) error
	meterProvider    otelmetric.MeterProvider
	tracer           oteltrace.Tracer
	requestCounter   otelmetric.Int64Counter
	latencyHistogram otelmetric.Float64Histogram
	host             string
	bypassEnabled    bool
}

func initializeOpenTelemetry(ctx context.Context, cfg config) (*otelRuntime, error) {
	host, err := os.Hostname()
	if err != nil || host == "" {
		host = "unknown"
	}
	noop := &otelRuntime{
		shutdown:      func(context.Context) error { return nil },
		meterProvider: otel.GetMeterProvider(),
		tracer:        otel.Tracer("gloadtest"),
		host:          host,
		bypassEnabled: cfg.enableBypass,
	}
	if !cfg.enableOTEL {
		return noop, nil
	}

	res := resource.NewWithAttributes(
		"",
		attribute.String("service.name", cfg.otelServiceName),
	)

	traceOpts := []gcptrace.Option{gcptrace.WithProjectID(cfg.otelProjectID)}
	if cfg.cloudTraceEndpoint != "" {
		traceOpts = append(traceOpts, gcptrace.WithTraceClientOptions([]option.ClientOption{option.WithEndpoint(cfg.cloudTraceEndpoint)}))
	}
	traceExp, err := gcptrace.New(traceOpts...)
	if err != nil {
		return nil, fmt.Errorf("create cloud trace exporter: %w", err)
	}
	traceProvider := trace.NewTracerProvider(
		trace.WithSampler(trace.TraceIDRatioBased(cfg.otelTraceSamplingFraction)),
		trace.WithBatcher(traceExp),
		trace.WithResource(res),
	)
	otel.SetTracerProvider(traceProvider)

	metricOpts := []gcpmetric.Option{
		gcpmetric.WithProjectID(cfg.otelProjectID),
		gcpmetric.WithMetricDescriptorTypeFormatter(func(m otelmetricdata.Metrics) string {
			prefix := strings.TrimSuffix(cfg.otelMetricPrefix, "/")
			name := strings.ReplaceAll(m.Name, ".", "/")
			return prefix + "/" + name
		}),
	}
	if cfg.monitoringEndpoint != "" {
		metricOpts = append(metricOpts, gcpmetric.WithMonitoringClientOptions(option.WithEndpoint(cfg.monitoringEndpoint)))
	}
	metricExp, err := gcpmetric.New(metricOpts...)
	if err != nil {
		return nil, fmt.Errorf("create cloud monitoring exporter: %w", err)
	}
	metricReader := metric.NewPeriodicReader(
		metricExp,
		metric.WithInterval(time.Duration(cfg.otelMetricExportIntervalSecond)*time.Second),
	)
	meterProvider := metric.NewMeterProvider(
		metric.WithResource(res),
		metric.WithReader(metricReader),
	)
	otel.SetMeterProvider(meterProvider)

	meter := meterProvider.Meter("gloadtest")
	requestCounter, err := meter.Int64Counter(
		"gop_count",
		otelmetric.WithDescription("Total requests processed by Go Spanner prober"),
		otelmetric.WithUnit("1"),
	)
	if err != nil {
		return nil, fmt.Errorf("create counter instrument: %w", err)
	}
	latencyHistogram, err := meter.Float64Histogram(
		"glatency",
		otelmetric.WithDescription("Request latency in milliseconds"),
		otelmetric.WithUnit("ms"),
		otelmetric.WithExplicitBucketBoundaries(
			0.1, 0.2, 0.5, 1, 2, 5, 10, 20, 50, 100, 200, 500, 1000,
		),
	)
	if err != nil {
		return nil, fmt.Errorf("create histogram instrument: %w", err)
	}
	if err := registerMemoryMetrics(meter, host, cfg); err != nil {
		return nil, err
	}

	return &otelRuntime{
		shutdown: func(shutdownCtx context.Context) error {
			return errors.Join(
				meterProvider.Shutdown(shutdownCtx),
				traceProvider.Shutdown(shutdownCtx),
			)
		},
		meterProvider:    meterProvider,
		tracer:           otel.Tracer("gloadtest"),
		requestCounter:   requestCounter,
		latencyHistogram: latencyHistogram,
		host:             host,
		bypassEnabled:    cfg.enableBypass,
	}, nil
}

func (o *otelRuntime) observeProbe(ctx context.Context, probeName string, latency time.Duration) {
	if o == nil || o.requestCounter == nil || o.latencyHistogram == nil {
		return
	}
	attrs := []attribute.KeyValue{
		attribute.String("method", probeName),
		attribute.Bool("bypass", o.bypassEnabled),
		attribute.String("host", o.host),
	}
	o.requestCounter.Add(ctx, 1, otelmetric.WithAttributes(attrs...))
	o.latencyHistogram.Record(ctx, float64(latency.Microseconds())/1000.0, otelmetric.WithAttributes(attrs...))
}

func recordSpanError(span oteltrace.Span, err error) {
	if span == nil || err == nil {
		return
	}
	span.SetStatus(otelcodes.Error, err.Error())
	span.RecordError(err)
}
