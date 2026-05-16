package main

import (
	"context"
	"errors"
	"fmt"
	"log"
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

var defaultLatencyMsBuckets = []float64{
	0.001, 0.002, 0.005, 0.01, 0.02, 0.03, 0.04, 0.05,
	0.06, 0.07, 0.08, 0.09, 0.1, 0.2, 0.3, 0.4,
	0.5, 0.6, 0.7, 0.8, 0.9, 1, 1.25, 1.5,
	1.75, 2, 2.25, 2.5, 2.75, 3, 3.25, 3.5,
	3.75, 4, 4.25, 4.5, 4.75, 5, 5.5, 6,
	6.5, 7, 7.5, 8, 8.5, 9, 9.5, 10,
	10.5, 11, 11.5, 12, 12.5, 13, 13.5, 14,
	14.5, 15, 16, 17, 18, 19, 20, 21,
	22, 23, 24, 25, 26, 27, 28, 29,
	30, 32.5, 35, 37.5, 40, 42.5, 45, 47.5,
	50, 55, 60, 65, 70, 75, 80, 85,
	90, 95, 100, 125, 150, 175, 200, 225,
	250, 275, 300, 325, 350, 375, 400, 425,
	450, 475, 500, 600, 700, 800, 900, 1000,
	1100, 1200, 1300, 1400, 1500, 1600, 1700, 1800,
	1900, 2000, 2500, 3000, 3500, 4000, 4500, 5000,
	6000, 7000, 8000, 9000, 10000, 20000, 30000, 40000,
	50000, 60000, 70000, 80000, 90000, 100000, 110000, 120000,
	150000, 180000, 210000, 240000, 270000, 300000,
}

func initializeOpenTelemetry(ctx context.Context, cfg config) (*otelRuntime, error) {
	otel.SetErrorHandler(otel.ErrorHandlerFunc(func(err error) {
		log.Printf("otel_error err=%v", err)
	}))

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

	resourceLocation := getEnvAny([]string{"OTEL_RESOURCE_LOCATION", "CLOUD_REGION", "GOOGLE_CLOUD_REGION"}, "global")
	resourceNamespace := getEnvAny([]string{"POD_NAMESPACE", "OTEL_RESOURCE_NAMESPACE"}, cfg.otelServiceName)
	if resourceNamespace == "" {
		resourceNamespace = cfg.otelServiceName
	}
	resourceTaskID := getEnvAny([]string{"POD_NAME", "HOSTNAME"}, host)
	if resourceTaskID == "" {
		resourceTaskID = host
	}
	res := resource.NewWithAttributes(
		"",
		attribute.String("service.name", cfg.otelServiceName),
		attribute.String("gcp.resource_type", "generic_task"),
		attribute.String("location", resourceLocation),
		attribute.String("namespace", resourceNamespace),
		attribute.String("job", cfg.otelServiceName),
		attribute.String("task_id", resourceTaskID),
	)
	fmt.Printf("otel_resource_debug monitored_resource=generic_task location=%s namespace=%s job=%s task_id=%s\n",
		resourceLocation,
		resourceNamespace,
		cfg.otelServiceName,
		resourceTaskID,
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
		gcpmetric.WithMonitoredResourceDescription("generic_task", []string{"location", "namespace", "job", "task_id"}),
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
	metricExporter := &loggingMetricExporter{
		Exporter: metricExp,
		debug:    cfg.otelExportDebug,
	}
	metricReader := metric.NewPeriodicReader(
		metricExporter,
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
		otelmetric.WithExplicitBucketBoundaries(defaultLatencyMsBuckets...),
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

type loggingMetricExporter struct {
	metric.Exporter
	debug bool
}

func (e *loggingMetricExporter) Export(ctx context.Context, rm *otelmetricdata.ResourceMetrics) error {
	names := metricNames(rm)
	if e.debug {
		log.Printf("otel_metric_export_start metric_count=%d metrics=%s", len(names), strings.Join(names, ","))
	}
	err := e.Exporter.Export(ctx, rm)
	if err != nil {
		log.Printf("otel_metric_export_error metric_count=%d metrics=%s err=%v", len(names), strings.Join(names, ","), err)
		return err
	}
	if e.debug {
		log.Printf("otel_metric_export_ok metric_count=%d metrics=%s", len(names), strings.Join(names, ","))
	}
	return nil
}

func (e *loggingMetricExporter) ForceFlush(ctx context.Context) error {
	err := e.Exporter.ForceFlush(ctx)
	if err != nil {
		log.Printf("otel_metric_force_flush_error err=%v", err)
		return err
	}
	if e.debug {
		log.Printf("otel_metric_force_flush_ok")
	}
	return nil
}

func (e *loggingMetricExporter) Shutdown(ctx context.Context) error {
	err := e.Exporter.Shutdown(ctx)
	if err != nil {
		log.Printf("otel_metric_shutdown_error err=%v", err)
		return err
	}
	if e.debug {
		log.Printf("otel_metric_shutdown_ok")
	}
	return nil
}

func metricNames(rm *otelmetricdata.ResourceMetrics) []string {
	if rm == nil {
		return nil
	}
	seen := make(map[string]bool)
	var names []string
	for _, scope := range rm.ScopeMetrics {
		for _, m := range scope.Metrics {
			if !seen[m.Name] {
				seen[m.Name] = true
				names = append(names, m.Name)
			}
		}
	}
	return names
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
