package main

import (
	"context"
	"log"
	"math/rand"
	"os"
	"os/signal"
	"reflect"
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
	log.Printf("probe=%s query_mode=%s start_qps=%.2f end_qps=%.2f step_qps_percent=%.2f qps_step_interval_s=%d burst_enabled=%t burst_after_s=%d qps_cycle_enabled=%t high_qps_hold_s=%d low_qps_hold_s=%d max_inflight=%d endpoint=%s insecure=%t db=%s num_rows=%d payload_size=%d max_staleness_s=%d direct_access=%t gcp_fallback=%t bypass=%t dcp_enabled=%t dcp_initial=%d dcp_min=%d dcp_max=%d dcp_max_rpc=%.2f dcp_min_rpc=%.2f pprof_enabled=%t pprof_addr=%s otel_enabled=%t otel_service=%s",
		cfg.probeType,
		cfg.queryMode,
		cfg.startQPS,
		cfg.endQPS,
		cfg.stepQPSPercent,
		cfg.qpsStepInterval,
		cfg.burstEnabled,
		cfg.burstAfterSeconds,
		cfg.qpsCycleEnabled,
		cfg.highQPSHoldSeconds,
		cfg.lowQPSHoldSeconds,
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
		cfg.enableDCP,
		cfg.dcpInitialChannels,
		cfg.dcpMinChannels,
		cfg.dcpMaxChannels,
		cfg.dcpMaxRPCPerChannel,
		cfg.dcpMinRPCPerChannel,
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
	applyDynamicChannelPoolConfig(&clientConfig, cfg)
	return spanner.NewClientWithConfig(ctx, cfg.databasePath, clientConfig, opts...)
}

func applyDynamicChannelPoolConfig(clientConfig *spanner.ClientConfig, cfg config) {
	if !cfg.enableDCP {
		return
	}
	v := reflect.ValueOf(clientConfig).Elem().FieldByName("DynamicChannelPoolConfig")
	if !v.IsValid() || !v.CanSet() {
		log.Printf("spanner_dcp_config_ignored=true reason=DynamicChannelPoolConfig_not_available")
		return
	}
	if v.Kind() == reflect.Ptr {
		if v.IsNil() {
			v.Set(reflect.New(v.Type().Elem()))
		}
		v = v.Elem()
	}
	if v.Kind() != reflect.Struct {
		log.Printf("spanner_dcp_config_ignored=true reason=DynamicChannelPoolConfig_not_struct kind=%s", v.Kind())
		return
	}
	if !setStructBool(v, "DCPEnabled", true) {
		log.Printf("spanner_dcp_config_ignored=true reason=DCPEnabled_not_available")
		return
	}
	setStructInt(v, "DCPInitialChannels", cfg.dcpInitialChannels)
	setStructInt(v, "DCPMinChannels", cfg.dcpMinChannels)
	setStructInt(v, "DCPMaxChannels", cfg.dcpMaxChannels)
	setStructFloat(v, "DCPMaxRPCPerChannel", cfg.dcpMaxRPCPerChannel)
	setStructFloat(v, "DCPMinRPCPerChannel", cfg.dcpMinRPCPerChannel)
}

func setStructBool(v reflect.Value, name string, value bool) bool {
	f := v.FieldByName(name)
	if f.IsValid() && f.CanSet() && f.Kind() == reflect.Bool {
		f.SetBool(value)
		return true
	}
	return false
}

func setStructInt(v reflect.Value, name string, value int) bool {
	if value == 0 {
		return true
	}
	f := v.FieldByName(name)
	if f.IsValid() && f.CanSet() && f.Kind() >= reflect.Int && f.Kind() <= reflect.Int64 {
		f.SetInt(int64(value))
		return true
	}
	log.Printf("spanner_dcp_field_ignored=true field=%s value=%d", name, value)
	return false
}

func setStructFloat(v reflect.Value, name string, value float64) bool {
	if value == 0 {
		return true
	}
	f := v.FieldByName(name)
	if f.IsValid() && f.CanSet() && (f.Kind() == reflect.Float32 || f.Kind() == reflect.Float64) {
		f.SetFloat(value)
		return true
	}
	log.Printf("spanner_dcp_field_ignored=true field=%s value=%f", name, value)
	return false
}
