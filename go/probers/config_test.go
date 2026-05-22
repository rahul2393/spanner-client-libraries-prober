package main

import (
	"strings"
	"testing"
)

func TestLoadConfigQPSCycleAliases(t *testing.T) {
	t.Setenv("LOAD_CYCLE_ENABLED", "true")
	t.Setenv("START_LOAD", "100")
	t.Setenv("END_LOAD", "1000")
	t.Setenv("STEP_LOAD_PERCENT", "25")
	t.Setenv("HIGH_LOAD_HOLD_SECONDS", "12")
	t.Setenv("LOW_LOAD_HOLD_SECONDS", "34")
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
	if !cfg.loadCycleEnabled {
		t.Fatal("loadCycleEnabled = false, want true")
	}
	if cfg.highLoadHoldSeconds != 12 || cfg.lowLoadHoldSeconds != 34 {
		t.Fatalf("hold seconds = high:%d low:%d, want high:12 low:34", cfg.highLoadHoldSeconds, cfg.lowLoadHoldSeconds)
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
	t.Setenv("START_LOAD", "100")
	t.Setenv("END_LOAD", "1000")

	cfg, err := loadConfig()
	if err != nil {
		t.Fatalf("loadConfig() failed: %v", err)
	}
	if !cfg.loadCycleEnabled || !cfg.burstEnabled {
		t.Fatalf("cycle/burst = %t/%t, want true/true", cfg.loadCycleEnabled, cfg.burstEnabled)
	}
}

func TestLoadConfigConcurrencyMode(t *testing.T) {
	t.Setenv("LOAD_MODE", "concurrency")
	t.Setenv("START_LOAD", "50")
	t.Setenv("END_LOAD", "600")
	t.Setenv("MAX_INFLIGHT", "600")

	cfg, err := loadConfig()
	if err != nil {
		t.Fatalf("loadConfig() failed: %v", err)
	}
	if cfg.loadMode != loadModeConcurrency {
		t.Fatalf("loadMode = %q, want %q", cfg.loadMode, loadModeConcurrency)
	}
}

func TestLoadConfigExplicitLoadSteps(t *testing.T) {
	t.Setenv("LOAD_MODE", "concurrency")
	t.Setenv("LOAD_STEPS", "10,50,100,50,10")
	t.Setenv("MAX_INFLIGHT", "100")
	t.Setenv("STEP_WARMUP_DISCARD_SECONDS", "30")

	cfg, err := loadConfig()
	if err != nil {
		t.Fatalf("loadConfig() failed: %v", err)
	}
	if got, want := cfg.loadSteps, []float64{10, 50, 100, 50, 10}; len(got) != len(want) {
		t.Fatalf("loadSteps len = %d, want %d: %v", len(got), len(want), got)
	} else {
		for i := range want {
			if got[i] != want[i] {
				t.Fatalf("loadSteps[%d] = %f, want %f; all=%v", i, got[i], want[i], got)
			}
		}
	}
	if cfg.startLoad != 10 || cfg.endLoad != 10 {
		t.Fatalf("start/end load = %.1f/%.1f, want first/last explicit step 10/10", cfg.startLoad, cfg.endLoad)
	}
	if cfg.stepWarmupDiscard != 30 {
		t.Fatalf("stepWarmupDiscard = %d, want 30", cfg.stepWarmupDiscard)
	}
}

func TestLoadConfigExplicitLoadStepsRejectParseError(t *testing.T) {
	t.Setenv("LOAD_STEPS", "10,nope,100")

	_, err := loadConfig()
	if err == nil || !strings.Contains(err.Error(), "invalid LOAD_STEPS value") {
		t.Fatalf("loadConfig error = %v, want invalid LOAD_STEPS value", err)
	}
}

func TestValidateConfigExplicitConcurrencyStepsMustFitMaxInflight(t *testing.T) {
	_, err := validateConfig(config{
		loadMode:                       loadModeConcurrency,
		startLoad:                      10,
		endLoad:                        10,
		loadSteps:                      []float64{10, 200},
		loadStepInterval:               1,
		numRows:                        1,
		payloadSize:                    1,
		maxInflight:                    100,
		maxStalenessSeconds:            1,
		logIntervalSeconds:             1,
		warmupCycles:                   0,
		otelTraceSamplingFraction:      1,
		otelMetricExportIntervalSecond: 1,
	})
	if err == nil || !strings.Contains(err.Error(), "LOAD_STEPS worker count") {
		t.Fatalf("validateConfig error = %v, want LOAD_STEPS worker count validation", err)
	}
}

func TestValidateConfigExplicitStepsRejectPercentRamp(t *testing.T) {
	_, err := validateConfig(config{
		loadMode:                       loadModeConcurrency,
		startLoad:                      10,
		endLoad:                        10,
		loadSteps:                      []float64{10, 20},
		stepLoadPercent:                50,
		loadStepInterval:               1,
		numRows:                        1,
		payloadSize:                    1,
		maxInflight:                    100,
		maxStalenessSeconds:            1,
		logIntervalSeconds:             1,
		warmupCycles:                   0,
		otelTraceSamplingFraction:      1,
		otelMetricExportIntervalSecond: 1,
	})
	if err == nil || !strings.Contains(err.Error(), "STEP_LOAD_PERCENT cannot be combined") {
		t.Fatalf("validateConfig error = %v, want LOAD_STEPS/STEP_LOAD_PERCENT conflict", err)
	}
}

func TestValidateConfigConcurrencyWorkersMustFitMaxInflight(t *testing.T) {
	_, err := validateConfig(config{
		loadMode:                       loadModeConcurrency,
		startLoad:                      50,
		endLoad:                        600,
		loadStepInterval:               1,
		numRows:                        1,
		payloadSize:                    1,
		maxInflight:                    100,
		maxStalenessSeconds:            1,
		logIntervalSeconds:             1,
		warmupCycles:                   0,
		otelTraceSamplingFraction:      1,
		otelMetricExportIntervalSecond: 1,
	})
	if err == nil || !strings.Contains(err.Error(), "END_LOAD worker count") {
		t.Fatalf("validateConfig error = %v, want END_LOAD worker count validation", err)
	}
}

func TestValidateConfigCycleRequiresEndAboveStart(t *testing.T) {
	_, err := validateConfig(config{
		startLoad:                      100,
		endLoad:                        100,
		stepLoadPercent:                50,
		loadStepInterval:               1,
		loadCycleEnabled:               true,
		lowLoadHoldSeconds:             1,
		numRows:                        1,
		payloadSize:                    1,
		maxInflight:                    1,
		maxStalenessSeconds:            1,
		logIntervalSeconds:             1,
		warmupCycles:                   0,
		otelTraceSamplingFraction:      1,
		otelMetricExportIntervalSecond: 1,
	})
	if err == nil || !strings.Contains(err.Error(), "END_LOAD must be greater") {
		t.Fatalf("validateConfig error = %v, want END_LOAD cycle validation", err)
	}
}

func TestValidateConfigNonBurstCycleRequiresStep(t *testing.T) {
	_, err := validateConfig(config{
		startLoad:                      100,
		endLoad:                        1000,
		loadStepInterval:               1,
		loadCycleEnabled:               true,
		lowLoadHoldSeconds:             1,
		numRows:                        1,
		payloadSize:                    1,
		maxInflight:                    1,
		maxStalenessSeconds:            1,
		logIntervalSeconds:             1,
		warmupCycles:                   0,
		otelTraceSamplingFraction:      1,
		otelMetricExportIntervalSecond: 1,
	})
	if err == nil || !strings.Contains(err.Error(), "STEP_LOAD_PERCENT") {
		t.Fatalf("validateConfig error = %v, want STEP_LOAD_PERCENT cycle validation", err)
	}
}

func TestValidateConfigBurstCycleRejectsZeroHighHold(t *testing.T) {
	_, err := validateConfig(config{
		startLoad:                      100,
		endLoad:                        1000,
		loadStepInterval:               1,
		burstEnabled:                   true,
		loadCycleEnabled:               true,
		burstAfterSeconds:              1,
		highLoadHoldSeconds:            0,
		numRows:                        1,
		payloadSize:                    1,
		maxInflight:                    1,
		maxStalenessSeconds:            1,
		logIntervalSeconds:             1,
		warmupCycles:                   0,
		otelTraceSamplingFraction:      1,
		otelMetricExportIntervalSecond: 1,
	})
	if err == nil || !strings.Contains(err.Error(), "HIGH_LOAD_HOLD_SECONDS") {
		t.Fatalf("validateConfig error = %v, want burst cycle high-hold validation", err)
	}
}

func TestValidateConfigNonBurstCycleRejectsZeroLowHold(t *testing.T) {
	_, err := validateConfig(config{
		startLoad:                      100,
		endLoad:                        1000,
		stepLoadPercent:                50,
		loadStepInterval:               1,
		loadCycleEnabled:               true,
		lowLoadHoldSeconds:             0,
		numRows:                        1,
		payloadSize:                    1,
		maxInflight:                    1,
		maxStalenessSeconds:            1,
		logIntervalSeconds:             1,
		warmupCycles:                   0,
		otelTraceSamplingFraction:      1,
		otelMetricExportIntervalSecond: 1,
	})
	if err == nil || !strings.Contains(err.Error(), "LOW_LOAD_HOLD_SECONDS") {
		t.Fatalf("validateConfig error = %v, want non-burst cycle low-hold validation", err)
	}
}
