package main

import (
	"context"
	"os"
	"strconv"
	"strings"

	"go.opentelemetry.io/otel/attribute"
	otelmetric "go.opentelemetry.io/otel/metric"
)

func registerMemoryMetrics(meter otelmetric.Meter, host string, cfg config) error {
	_, err := meter.Int64ObservableGauge(
		"container_memory_bytes",
		otelmetric.WithDescription("Current container memory usage from cgroup"),
		otelmetric.WithUnit("By"),
		otelmetric.WithInt64Callback(func(ctx context.Context, observer otelmetric.Int64Observer) error {
			if value, ok := readContainerMemoryBytes(); ok {
				observer.Observe(value, otelmetric.WithAttributes(
					attribute.String("host", host),
					attribute.String("probe_type", cfg.probeType),
				))
			}
			return nil
		}),
	)
	return err
}

func readContainerMemoryBytes() (int64, bool) {
	for _, path := range []string{
		"/sys/fs/cgroup/memory.current",
		"/sys/fs/cgroup/memory/memory.usage_in_bytes",
	} {
		b, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		v, err := strconv.ParseInt(strings.TrimSpace(string(b)), 10, 64)
		if err == nil {
			return v, true
		}
	}
	return 0, false
}
