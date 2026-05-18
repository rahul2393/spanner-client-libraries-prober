package main

import (
	"context"
	"testing"
)

func TestNewProbeTestNoopDoesNotRequireSpannerClient(t *testing.T) {
	p, err := newProbe(nil, config{probeType: "test_noop"})
	if err != nil {
		t.Fatalf("newProbe(test_noop) failed: %v", err)
	}
	noop, ok := p.(*noopProbe)
	if !ok {
		t.Fatalf("newProbe(test_noop) = %T, want *noopProbe", p)
	}
	if noop.Name() != "test_noop" {
		t.Fatalf("noop name = %q, want test_noop", noop.Name())
	}
	if err := noop.Probe(context.Background()); err != nil {
		t.Fatalf("noop probe failed: %v", err)
	}
	if got := noop.Count(); got != 1 {
		t.Fatalf("noop count = %d, want 1", got)
	}
}

func TestTestNoopProbeConfigCases(t *testing.T) {
	cases := []struct {
		name string
		cfg  config
	}{
		{
			name: "no dcp steady",
			cfg:  validTestNoopConfig(false),
		},
		{
			name: "dcp steady",
			cfg:  validTestNoopConfig(true),
		},
		{
			name: "dcp ramp cycle",
			cfg: func() config {
				cfg := validTestNoopConfig(true)
				cfg.endLoad = 1000
				cfg.stepLoadPercent = 50
				cfg.loadCycleEnabled = true
				cfg.lowLoadHoldSeconds = 1
				return cfg
			}(),
		},
		{
			name: "dcp burst cycle",
			cfg: func() config {
				cfg := validTestNoopConfig(true)
				cfg.endLoad = 1000
				cfg.burstEnabled = true
				cfg.burstAfterSeconds = 0
				cfg.loadCycleEnabled = true
				cfg.highLoadHoldSeconds = 1
				return cfg
			}(),
		},
		{
			name: "no dcp burst cycle",
			cfg: func() config {
				cfg := validTestNoopConfig(false)
				cfg.endLoad = 1000
				cfg.burstEnabled = true
				cfg.burstAfterSeconds = 0
				cfg.loadCycleEnabled = true
				cfg.highLoadHoldSeconds = 1
				return cfg
			}(),
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := validateConfig(tc.cfg); err != nil {
				t.Fatalf("validateConfig() failed: %v", err)
			}
			p, err := newProbe(nil, tc.cfg)
			if err != nil {
				t.Fatalf("newProbe(test_noop) failed: %v", err)
			}
			noop, ok := p.(*noopProbe)
			if !ok {
				t.Fatalf("newProbe(test_noop) = %T, want *noopProbe", p)
			}
			if err := noop.Probe(context.Background()); err != nil {
				t.Fatalf("noop probe failed: %v", err)
			}
		})
	}
}

func TestTestNoopProbeInjectedErrors(t *testing.T) {
	p := &noopProbe{errorEvery: 2}
	if err := p.Probe(context.Background()); err != nil {
		t.Fatalf("first noop probe failed: %v", err)
	}
	if err := p.Probe(context.Background()); err == nil {
		t.Fatal("second noop probe succeeded, want injected error")
	}
	if got := p.Count(); got != 2 {
		t.Fatalf("noop count = %d, want 2", got)
	}
}

func validTestNoopConfig(enableDCP bool) config {
	return config{
		probeType:                      "test_noop",
		startLoad:                      100,
		loadStepInterval:               1,
		burstAfterSeconds:              0,
		highLoadHoldSeconds:            1,
		lowLoadHoldSeconds:             1,
		numRows:                        1,
		payloadSize:                    1,
		maxInflight:                    1,
		maxStalenessSeconds:            1,
		queryMode:                      "normal",
		logIntervalSeconds:             1,
		warmupCycles:                   0,
		enableDCP:                      enableDCP,
		otelTraceSamplingFraction:      1,
		otelMetricExportIntervalSecond: 1,
	}
}
