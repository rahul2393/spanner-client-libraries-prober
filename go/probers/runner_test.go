package main

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

func TestStepUpQPSClampsAtEnd(t *testing.T) {
	if got := stepUpQPS(100, 500, 50); got != 150 {
		t.Fatalf("stepUpQPS(100,500,50) = %f, want 150", got)
	}
	if got := stepUpQPS(400, 500, 50); got != 500 {
		t.Fatalf("stepUpQPS(400,500,50) = %f, want 500", got)
	}
	if got := stepUpQPS(400, 0, 50); got != 600 {
		t.Fatalf("stepUpQPS(400,0,50) = %f, want 600", got)
	}
}

func TestStepDownQPSClampsAtStart(t *testing.T) {
	if got := stepDownQPS(1000, 200, 50); got != 500 {
		t.Fatalf("stepDownQPS(1000,200,50) = %f, want 500", got)
	}
	if got := stepDownQPS(300, 200, 50); got != 200 {
		t.Fatalf("stepDownQPS(300,200,50) = %f, want 200", got)
	}
	if got := stepDownQPS(1000, 200, 100); got != 200 {
		t.Fatalf("stepDownQPS(1000,200,100) = %f, want 200", got)
	}
}

func TestRampCycleQPSSequence(t *testing.T) {
	cfg := config{startQPS: 200, endQPS: 1000, stepQPSPercent: 50}
	qps := cfg.startQPS

	qps = stepUpQPS(qps, cfg.endQPS, cfg.stepQPSPercent)
	if qps != 300 {
		t.Fatalf("first step up = %f, want 300", qps)
	}
	qps = stepUpQPS(qps, cfg.endQPS, cfg.stepQPSPercent)
	if qps != 450 {
		t.Fatalf("second step up = %f, want 450", qps)
	}
	for qps < cfg.endQPS {
		qps = stepUpQPS(qps, cfg.endQPS, cfg.stepQPSPercent)
	}
	if qps != cfg.endQPS {
		t.Fatalf("ramp cap = %f, want %f", qps, cfg.endQPS)
	}

	qps = stepDownQPS(qps, cfg.startQPS, cfg.stepQPSPercent)
	if qps != 500 {
		t.Fatalf("first step down = %f, want 500", qps)
	}
	qps = stepDownQPS(qps, cfg.startQPS, cfg.stepQPSPercent)
	if qps != 250 {
		t.Fatalf("second step down = %f, want 250", qps)
	}
	qps = stepDownQPS(qps, cfg.startQPS, cfg.stepQPSPercent)
	if qps != cfg.startQPS {
		t.Fatalf("step down floor = %f, want %f", qps, cfg.startQPS)
	}
}

func TestDispatchDelay(t *testing.T) {
	if got := dispatchDelay(0); got != time.Second {
		t.Fatalf("dispatchDelay(0) = %s, want 1s", got)
	}
	if got := dispatchDelay(1000); got != time.Millisecond {
		t.Fatalf("dispatchDelay(1000) = %s, want 1ms", got)
	}
	if fast, slow := dispatchDelay(200), dispatchDelay(100); fast >= slow {
		t.Fatalf("dispatchDelay monotonic failed: fast=%s slow=%s", fast, slow)
	}
}

func TestRunBurstCycleUsesCompressedTestQPSWindow(t *testing.T) {
	unit := 250 * time.Millisecond
	started := time.Now()
	noop := &noopProbe{
		buckets: make([]atomic.Int64, 4),
		bucketFn: func() int {
			return int(time.Since(started) / unit)
		},
	}
	cfg := validTestNoopConfig(true)
	cfg.startQPS = 10
	cfg.endQPS = 500
	cfg.burstEnabled = true
	cfg.burstAfterSeconds = 1
	cfg.qpsCycleEnabled = true
	cfg.highQPSHoldSeconds = 1
	cfg.maxInflight = 1000
	cfg.logIntervalSeconds = 10

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		runWithOptions(ctx, cfg, noop, nil, runOptions{timeUnit: unit})
	}()
	time.Sleep(740 * time.Millisecond)
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("runWithOptions did not stop")
	}
	time.Sleep(25 * time.Millisecond)

	counts := noop.BucketCounts()
	lowBefore := counts[0]
	high := counts[1]
	lowAfter := counts[2]
	if lowBefore < 5 || lowBefore > 25 {
		t.Fatalf("low-before bucket count = %d, want roughly 10; all buckets=%v", lowBefore, counts)
	}
	if high < 100 || high < lowBefore*8 {
		t.Fatalf("high bucket count = %d, want burst much higher than low-before=%d; all buckets=%v", high, lowBefore, counts)
	}
	if lowAfter < 5 || lowAfter > 40 || lowAfter*5 > high {
		t.Fatalf("low-after bucket count = %d, want reset near low and much lower than high=%d; all buckets=%v", lowAfter, high, counts)
	}
}
