package main

import (
	"context"
	"log"
	"math/rand"
	"os"
	"os/signal"
	"syscall"
	"time"

	"cloud.google.com/go/spanner"
	"google.golang.org/api/option"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func main() {
	rand.Seed(time.Now().UnixNano())
	cfg, err := loadConfig()
	if err != nil {
		log.Fatalf("invalid configuration: %v", err)
	}
	printConfig(cfg)
	startPprofServer(cfg)

	rootCtx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	otelState, err := initializeOpenTelemetry(rootCtx, cfg)
	if err != nil {
		log.Fatalf("failed to initialize OpenTelemetry: %v", err)
	}
	defer func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		if shutdownErr := otelState.shutdown(shutdownCtx); shutdownErr != nil {
			log.Printf("OpenTelemetry shutdown error: %v", shutdownErr)
		}
	}()

	client, err := newClient(rootCtx, cfg, otelState)
	if err != nil {
		log.Fatalf("failed to create spanner client: %v", err)
	}
	defer client.Close()

	p, err := newProbe(client, cfg)
	if err != nil {
		log.Fatalf("failed to create probe: %v", err)
	}
	if err := warmup(rootCtx, p, cfg.warmupCycles); err != nil {
		log.Fatalf("warmup failed: %v", err)
	}
	log.Printf("warmup complete (%d cycles), starting probe loop", cfg.warmupCycles)
	run(rootCtx, cfg, p, otelState)
}

func printConfig(cfg config) {
	log.Printf("probe=%s query_mode=%s start_qps=%.2f end_qps=%.2f step_qps_percent=%.2f qps_step_interval_s=%d burst_enabled=%t burst_after_s=%d max_inflight=%d endpoint=%s insecure=%t db=%s num_rows=%d payload_size=%d max_staleness_s=%d direct_access=%t gcp_fallback=%t bypass=%t pprof_enabled=%t pprof_addr=%s otel_enabled=%t otel_service=%s",
		cfg.probeType,
		cfg.queryMode,
		cfg.startQPS,
		cfg.endQPS,
		cfg.stepQPSPercent,
		cfg.qpsStepInterval,
		cfg.burstEnabled,
		cfg.burstAfterSeconds,
		cfg.maxInflight,
		cfg.endpoint,
		cfg.insecure,
		cfg.databasePath,
		cfg.numRows,
		cfg.payloadSize,
		cfg.maxStalenessSeconds,
		cfg.enableDirectAccess,
		cfg.enableGcpFallback,
		cfg.enableBypass,
		cfg.enablePprof,
		cfg.pprofAddr,
		cfg.enableOTEL,
		cfg.otelServiceName,
	)
}

func newClient(ctx context.Context, cfg config, otelState *otelRuntime) (*spanner.Client, error) {
	opts := []option.ClientOption{}
	if cfg.endpoint != "" {
		opts = append(opts, option.WithEndpoint(cfg.endpoint))
	}
	if cfg.insecure {
		opts = append(opts, option.WithoutAuthentication())
		opts = append(opts, option.WithGRPCDialOption(
			grpc.WithTransportCredentials(insecure.NewCredentials()),
		))
	}
	clientConfig := spanner.ClientConfig{
		SessionPoolConfig:          spanner.DefaultSessionPoolConfig,
		DisableRouteToLeader:       false,
		OpenTelemetryMeterProvider: otelState.meterProvider,
		EnableEndToEndTracing:      cfg.enableSpannerEndToEndTracing,
		EnableDirectAccess:         cfg.enableDirectAccess,
	}
	return spanner.NewClientWithConfig(ctx, cfg.databasePath, clientConfig, opts...)
}
