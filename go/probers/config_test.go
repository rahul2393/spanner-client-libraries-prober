package main

import (
	"strings"
	"testing"
)

func TestLoadConfigQPSCycleAliases(t *testing.T) {
	t.Setenv("QPS_CYCLE_ENABLED", "true")
	t.Setenv("START_QPS", "100")
	t.Setenv("END_QPS", "1000")
	t.Setenv("STEP_QPS_PERCENT", "25")
	t.Setenv("HIGH_QPS_HOLD_SECONDS", "12")
	t.Setenv("LOW_QPS_HOLD_SECONDS", "34")
	t.Setenv("SPANNER_DCP_ENABLED", "true")
	t.Setenv("SPANNER_DCP_INITIAL_CHANNELS", "4")
	t.Setenv("SPANNER_DCP_MIN_CHANNELS", "1")
	t.Setenv("SPANNER_DCP_MAX_CHANNELS", "200")
	t.Setenv("SPANNER_DCP_MAX_RPC_PER_CHANNEL", "50")
	t.Setenv("SPANNER_DCP_MIN_RPC_PER_CHANNEL", "5")

	cfg, err := loadConfig()
	if err != nil {
		t.Fatalf("loadConfig() failed: %v", err)
	}
	if !cfg.qpsCycleEnabled {
		t.Fatal("qpsCycleEnabled = false, want true")
	}
	if cfg.highQPSHoldSeconds != 12 || cfg.lowQPSHoldSeconds != 34 {
		t.Fatalf("hold seconds = high:%d low:%d, want high:12 low:34", cfg.highQPSHoldSeconds, cfg.lowQPSHoldSeconds)
	}
	if !cfg.enableDCP || cfg.dcpInitialChannels != 4 || cfg.dcpMinChannels != 1 || cfg.dcpMaxChannels != 200 {
		t.Fatalf("DCP config not parsed: %+v", cfg)
	}
	if cfg.dcpMaxRPCPerChannel != 50 || cfg.dcpMinRPCPerChannel != 5 {
		t.Fatalf("DCP RPC thresholds = max:%f min:%f, want 50/5", cfg.dcpMaxRPCPerChannel, cfg.dcpMinRPCPerChannel)
	}
}

func TestLoadConfigCycleAliasAndBurstMode(t *testing.T) {
	t.Setenv("CYCLE_ENABLED", "true")
	t.Setenv("BURST_MODE", "true")
	t.Setenv("START_QPS", "100")
	t.Setenv("END_QPS", "1000")

	cfg, err := loadConfig()
	if err != nil {
		t.Fatalf("loadConfig() failed: %v", err)
	}
	if !cfg.qpsCycleEnabled || !cfg.burstEnabled {
		t.Fatalf("cycle/burst = %t/%t, want true/true", cfg.qpsCycleEnabled, cfg.burstEnabled)
	}
}

func TestLoadConfigConcurrencyMode(t *testing.T) {
	t.Setenv("LOAD_MODE", "concurrency")
	t.Setenv("START_QPS", "50")
	t.Setenv("END_QPS", "600")
	t.Setenv("MAX_INFLIGHT", "600")

	cfg, err := loadConfig()
	if err != nil {
		t.Fatalf("loadConfig() failed: %v", err)
	}
	if cfg.loadMode != loadModeConcurrency {
		t.Fatalf("loadMode = %q, want %q", cfg.loadMode, loadModeConcurrency)
	}
}

func TestValidateConfigConcurrencyWorkersMustFitMaxInflight(t *testing.T) {
	_, err := validateConfig(config{
		loadMode:                       loadModeConcurrency,
		startQPS:                       50,
		endQPS:                         600,
		qpsStepInterval:                1,
		numRows:                        1,
		payloadSize:                    1,
		maxInflight:                    100,
		maxStalenessSeconds:            1,
		logIntervalSeconds:             1,
		warmupCycles:                   0,
		otelTraceSamplingFraction:      1,
		otelMetricExportIntervalSecond: 1,
	})
	if err == nil || !strings.Contains(err.Error(), "END_QPS worker count") {
		t.Fatalf("validateConfig error = %v, want END_QPS worker count validation", err)
	}
}

func TestValidateConfigCycleRequiresEndAboveStart(t *testing.T) {
	_, err := validateConfig(config{
		startQPS:                       100,
		endQPS:                         100,
		stepQPSPercent:                 50,
		qpsStepInterval:                1,
		qpsCycleEnabled:                true,
		lowQPSHoldSeconds:              1,
		numRows:                        1,
		payloadSize:                    1,
		maxInflight:                    1,
		maxStalenessSeconds:            1,
		logIntervalSeconds:             1,
		warmupCycles:                   0,
		otelTraceSamplingFraction:      1,
		otelMetricExportIntervalSecond: 1,
	})
	if err == nil || !strings.Contains(err.Error(), "END_QPS must be greater") {
		t.Fatalf("validateConfig error = %v, want END_QPS cycle validation", err)
	}
}

func TestValidateConfigNonBurstCycleRequiresStep(t *testing.T) {
	_, err := validateConfig(config{
		startQPS:                       100,
		endQPS:                         1000,
		qpsStepInterval:                1,
		qpsCycleEnabled:                true,
		lowQPSHoldSeconds:              1,
		numRows:                        1,
		payloadSize:                    1,
		maxInflight:                    1,
		maxStalenessSeconds:            1,
		logIntervalSeconds:             1,
		warmupCycles:                   0,
		otelTraceSamplingFraction:      1,
		otelMetricExportIntervalSecond: 1,
	})
	if err == nil || !strings.Contains(err.Error(), "STEP_QPS_PERCENT") {
		t.Fatalf("validateConfig error = %v, want STEP_QPS_PERCENT cycle validation", err)
	}
}

func TestValidateConfigBurstCycleRejectsZeroHighHold(t *testing.T) {
	_, err := validateConfig(config{
		startQPS:                       100,
		endQPS:                         1000,
		qpsStepInterval:                1,
		burstEnabled:                   true,
		qpsCycleEnabled:                true,
		burstAfterSeconds:              1,
		highQPSHoldSeconds:             0,
		numRows:                        1,
		payloadSize:                    1,
		maxInflight:                    1,
		maxStalenessSeconds:            1,
		logIntervalSeconds:             1,
		warmupCycles:                   0,
		otelTraceSamplingFraction:      1,
		otelMetricExportIntervalSecond: 1,
	})
	if err == nil || !strings.Contains(err.Error(), "HIGH_QPS_HOLD_SECONDS") {
		t.Fatalf("validateConfig error = %v, want burst cycle high-hold validation", err)
	}
}

func TestValidateConfigNonBurstCycleRejectsZeroLowHold(t *testing.T) {
	_, err := validateConfig(config{
		startQPS:                       100,
		endQPS:                         1000,
		stepQPSPercent:                 50,
		qpsStepInterval:                1,
		qpsCycleEnabled:                true,
		lowQPSHoldSeconds:              0,
		numRows:                        1,
		payloadSize:                    1,
		maxInflight:                    1,
		maxStalenessSeconds:            1,
		logIntervalSeconds:             1,
		warmupCycles:                   0,
		otelTraceSamplingFraction:      1,
		otelMetricExportIntervalSecond: 1,
	})
	if err == nil || !strings.Contains(err.Error(), "LOW_QPS_HOLD_SECONDS") {
		t.Fatalf("validateConfig error = %v, want non-burst cycle low-hold validation", err)
	}
}
