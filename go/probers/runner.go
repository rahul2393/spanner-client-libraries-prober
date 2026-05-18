package main

import (
	"context"
	"log"
	"math"
	"sync/atomic"
	"time"

	"go.opentelemetry.io/otel/attribute"
	oteltrace "go.opentelemetry.io/otel/trace"
)

type metrics struct {
	okCount           atomic.Int64
	errorCount        atomic.Int64
	droppedCount      atomic.Int64
	okInterval        atomic.Int64
	errorInterval     atomic.Int64
	droppedInterval   atomic.Int64
	latencyIntervalUs atomic.Int64
	totalLatencyUs    atomic.Int64
	currentInflight   atomic.Int64
	maxInflightSeen   atomic.Int64
	inflightSampleSum atomic.Int64
	inflightSamples   atomic.Int64
}

type runOptions struct {
	timeUnit time.Duration
}

func defaultRunOptions() runOptions {
	return runOptions{timeUnit: time.Second}
}

func (o runOptions) unit() time.Duration {
	if o.timeUnit <= 0 {
		return time.Second
	}
	return o.timeUnit
}

func run(ctx context.Context, cfg config, p probe, otelState *otelRuntime) {
	runWithOptions(ctx, cfg, p, otelState, defaultRunOptions())
}

func runWithOptions(ctx context.Context, cfg config, p probe, otelState *otelRuntime, opts runOptions) {
	unit := opts.unit()
	statsTicker := time.NewTicker(time.Duration(cfg.logIntervalSeconds) * unit)
	defer statsTicker.Stop()

	targetLoad := newTargetLoad(cfg.startLoad)
	startLoadController(ctx, cfg, targetLoad, unit)

	m := &metrics{}
	start := time.Now()

	if cfg.loadMode == loadModeConcurrency {
		runConcurrencyMode(ctx, cfg, p, otelState, statsTicker, targetLoad, m, start)
		return
	}

	runQPSMode(ctx, cfg, p, otelState, statsTicker, targetLoad, m, start, unit)
}

func runQPSMode(ctx context.Context, cfg config, p probe, otelState *otelRuntime, statsTicker *time.Ticker, targetLoad *targetLoadState, m *metrics, start time.Time, unit time.Duration) {
	sem := make(chan struct{}, cfg.maxInflight)
	dispatchTick := qpsDispatchTick(unit)
	dispatchTicker := time.NewTicker(dispatchTick)
	defer dispatchTicker.Stop()
	last := time.Now()
	tokens := 0.0
	for {
		select {
		case <-ctx.Done():
			log.Printf("stopping (%v)", ctx.Err())
			printSummary(m, time.Since(start), targetLoad.Load(), m.currentInflight.Load(), cfg, true)
			return
		case <-statsTicker.C:
			printSummary(m, time.Since(start), targetLoad.Load(), m.currentInflight.Load(), cfg, false)
		case now := <-dispatchTicker.C:
			target := targetLoad.Load()
			elapsed := now.Sub(last)
			last = now
			tokens += target * float64(elapsed) / float64(unit)
			if maxTokens := float64(cfg.maxInflight); tokens > maxTokens {
				tokens = maxTokens
			}
			toDispatch := int(tokens)
			if toDispatch == 0 {
				continue
			}
			tokens -= float64(toDispatch)
			for i := 0; i < toDispatch; i++ {
				select {
				case sem <- struct{}{}:
					go func() {
						defer func() { <-sem }()
						executeProbe(ctx, p, otelState, m)
					}()
				default:
					m.droppedCount.Add(1)
					m.droppedInterval.Add(1)
				}
			}
		}
	}
}

func qpsDispatchTick(unit time.Duration) time.Duration {
	tick := unit / 100
	if tick < time.Millisecond {
		return time.Millisecond
	}
	return tick
}

func runConcurrencyMode(ctx context.Context, cfg config, p probe, otelState *otelRuntime, statsTicker *time.Ticker, targetWorkers *targetLoadState, m *metrics, start time.Time) {
	for i := 0; i < cfg.maxInflight; i++ {
		workerNumber := i + 1
		go func() {
			for {
				select {
				case <-ctx.Done():
					return
				default:
				}
				if workerNumber <= targetWorkerCount(targetWorkers.Load(), cfg) {
					executeProbe(ctx, p, otelState, m)
					continue
				}
				if !sleepOrDone(ctx, 100*time.Millisecond) {
					return
				}
			}
		}()
	}

	for {
		select {
		case <-ctx.Done():
			log.Printf("stopping (%v)", ctx.Err())
			printSummary(m, time.Since(start), targetWorkers.Load(), m.currentInflight.Load(), cfg, true)
			return
		case <-statsTicker.C:
			printSummary(m, time.Since(start), targetWorkers.Load(), m.currentInflight.Load(), cfg, false)
		}
	}
}

func executeProbe(ctx context.Context, p probe, otelState *otelRuntime, m *metrics) {
	inflight := m.currentInflight.Add(1)
	updateMaxInt64(&m.maxInflightSeen, inflight)
	recordInflightSample(m, inflight)
	defer func() {
		recordInflightSample(m, m.currentInflight.Add(-1))
	}()

	probeCtx := ctx
	var span oteltrace.Span
	if otelState != nil && otelState.tracer != nil {
		probeCtx, span = otelState.tracer.Start(
			ctx,
			"probe."+p.Name(),
			oteltrace.WithAttributes(
				attribute.String("probe.type", p.Name()),
				attribute.Bool("probe.bypass_enabled", otelState.bypassEnabled),
				attribute.String("probe.host", otelState.host),
			),
		)
	}

	callStart := time.Now()
	err := p.Probe(probeCtx)
	latency := time.Since(callStart)
	latencyMicros := latency.Microseconds()
	m.totalLatencyUs.Add(latencyMicros)
	m.latencyIntervalUs.Add(latencyMicros)

	if span != nil {
		recordSpanError(span, err)
		span.SetAttributes(attribute.Float64("probe.latency_ms", float64(latency.Microseconds())/1000.0))
		span.End()
	}
	if otelState != nil {
		otelState.observeProbe(probeCtx, p.Name(), latency)
	}

	if err != nil {
		m.errorCount.Add(1)
		m.errorInterval.Add(1)
		log.Printf("probe_error type=%s err=%v", p.Name(), err)
		return
	}
	m.okCount.Add(1)
	m.okInterval.Add(1)
}

func recordInflightSample(m *metrics, inflight int64) {
	m.inflightSampleSum.Add(inflight)
	m.inflightSamples.Add(1)
}

func updateMaxInt64(v *atomic.Int64, candidate int64) {
	for {
		current := v.Load()
		if candidate <= current || v.CompareAndSwap(current, candidate) {
			return
		}
	}
}

func targetWorkerCount(target float64, cfg config) int {
	workers := int(math.Round(target))
	if workers < 0 {
		return 0
	}
	if workers > cfg.maxInflight {
		return cfg.maxInflight
	}
	return workers
}

func loadUnit(cfg config) string {
	if cfg.loadMode == loadModeConcurrency {
		return "workers"
	}
	return "qps"
}

type targetLoadState struct {
	value atomic.Uint64
}

func newTargetLoad(qps float64) *targetLoadState {
	s := &targetLoadState{}
	s.Store(qps)
	return s
}

func (s *targetLoadState) Load() float64 {
	return math.Float64frombits(s.value.Load())
}

func (s *targetLoadState) Store(qps float64) {
	s.value.Store(math.Float64bits(qps))
}

func dispatchDelay(qps float64) time.Duration {
	return dispatchDelayForUnit(qps, time.Second)
}

func dispatchDelayForUnit(qps float64, unit time.Duration) time.Duration {
	if qps <= 0 {
		return unit
	}
	delay := time.Duration(float64(unit) / qps)
	if delay <= 0 {
		return time.Nanosecond
	}
	return delay
}

func startLoadController(ctx context.Context, cfg config, targetLoad *targetLoadState, unit time.Duration) {
	if cfg.burstEnabled {
		if cfg.loadCycleEnabled {
			startBurstLoadCycle(ctx, cfg, targetLoad, unit)
			return
		}
		go func() {
			timer := time.NewTimer(time.Duration(cfg.burstAfterSeconds) * unit)
			defer timer.Stop()
			select {
			case <-ctx.Done():
				return
			case <-timer.C:
				if cfg.endLoad > 0 {
					targetLoad.Store(cfg.endLoad)
					log.Printf("burst_%s target_%s=%.2f", loadUnit(cfg), loadUnit(cfg), cfg.endLoad)
				}
				startLoadRamp(ctx, cfg, targetLoad, unit)
			}
		}()
		return
	}
	if cfg.loadCycleEnabled {
		startRampLoadCycle(ctx, cfg, targetLoad, unit)
		return
	}
	startLoadRamp(ctx, cfg, targetLoad, unit)
}

func startBurstLoadCycle(ctx context.Context, cfg config, targetLoad *targetLoadState, unit time.Duration) {
	go func() {
		for {
			if !sleepOrDone(ctx, time.Duration(cfg.burstAfterSeconds)*unit) {
				return
			}
			targetLoad.Store(cfg.endLoad)
			log.Printf("burst_%s target_%s=%.2f hold_seconds=%d", loadUnit(cfg), loadUnit(cfg), cfg.endLoad, cfg.highLoadHoldSeconds)
			if !sleepOrDone(ctx, time.Duration(cfg.highLoadHoldSeconds)*unit) {
				return
			}
			targetLoad.Store(cfg.startLoad)
			log.Printf("reset_%s target_%s=%.2f hold_seconds=%d", loadUnit(cfg), loadUnit(cfg), cfg.startLoad, cfg.burstAfterSeconds)
		}
	}()
}

func startRampLoadCycle(ctx context.Context, cfg config, targetLoad *targetLoadState, unit time.Duration) {
	go func() {
		direction := 1
		ticker := time.NewTicker(time.Duration(cfg.loadStepInterval) * unit)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				current := targetLoad.Load()
				if direction > 0 {
					next := stepUpQPS(current, cfg.endLoad, cfg.stepLoadPercent)
					if next == cfg.endLoad {
						direction = -1
					}
					targetLoad.Store(next)
					log.Printf("step_up_%s old_%s=%.2f new_%s=%.2f", loadUnit(cfg), loadUnit(cfg), current, loadUnit(cfg), next)
					continue
				}

				next := stepDownQPS(current, cfg.startLoad, cfg.stepLoadPercent)
				targetLoad.Store(next)
				log.Printf("step_down_%s old_%s=%.2f new_%s=%.2f", loadUnit(cfg), loadUnit(cfg), current, loadUnit(cfg), next)
				if next == cfg.startLoad {
					log.Printf("low_%s_hold target_%s=%.2f hold_seconds=%d", loadUnit(cfg), loadUnit(cfg), cfg.startLoad, cfg.lowLoadHoldSeconds)
					if !sleepOrDone(ctx, time.Duration(cfg.lowLoadHoldSeconds)*unit) {
						return
					}
					direction = 1
				}
			}
		}
	}()
}

func startLoadRamp(ctx context.Context, cfg config, targetLoad *targetLoadState, unit time.Duration) {
	if cfg.stepLoadPercent <= 0 {
		return
	}
	go func() {
		ticker := time.NewTicker(time.Duration(cfg.loadStepInterval) * unit)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				current := targetLoad.Load()
				if cfg.endLoad > 0 && current >= cfg.endLoad {
					continue
				}
				next := stepUpQPS(current, cfg.endLoad, cfg.stepLoadPercent)
				targetLoad.Store(next)
				log.Printf("step_%s old_%s=%.2f new_%s=%.2f", loadUnit(cfg), loadUnit(cfg), current, loadUnit(cfg), next)
			}
		}
	}()
}

func stepUpQPS(current, end, percent float64) float64 {
	next := current * (1.0 + percent/100.0)
	if end > 0 && next > end {
		return end
	}
	return next
}

func stepDownQPS(current, start, percent float64) float64 {
	if percent >= 100 {
		return start
	}
	next := current * (1.0 - percent/100.0)
	if next < start {
		return start
	}
	return next
}

func sleepOrDone(ctx context.Context, d time.Duration) bool {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}

func warmup(ctx context.Context, p probe, cycles int) error {
	for i := 0; i < cycles; i++ {
		if err := p.Probe(ctx); err != nil {
			return err
		}
	}
	return nil
}

func printSummary(m *metrics, elapsed time.Duration, targetLoad float64, inflight int64, cfg config, final bool) {
	ok := m.okCount.Load()
	errs := m.errorCount.Load()
	dropped := m.droppedCount.Load()
	total := ok + errs
	avgLatMic := int64(0)
	if total > 0 {
		avgLatMic = m.totalLatencyUs.Load() / total
	}
	achievedQPS := 0.0
	if elapsed > 0 {
		achievedQPS = float64(total) / elapsed.Seconds()
	}
	maxInflightSeen := m.maxInflightSeen.Swap(inflight)
	inflightSamples := m.inflightSamples.Swap(1)
	inflightSampleSum := m.inflightSampleSum.Swap(inflight)
	avgInflight := float64(inflight)
	if inflightSamples > 0 {
		avgInflight = float64(inflightSampleSum) / float64(inflightSamples)
	}
	if final {
		if cfg.loadMode == loadModeConcurrency {
			log.Printf("stats elapsed=%s load_mode=concurrency target_workers=%d ok=%d err=%d dropped=%d avg_latency_us=%d achieved_qps=%.2f inflight=%d avg_inflight=%.2f max_inflight_seen=%d max_inflight=%d final=true", elapsed.Truncate(time.Second), targetWorkerCount(targetLoad, cfg), ok, errs, dropped, avgLatMic, achievedQPS, inflight, avgInflight, maxInflightSeen, cfg.maxInflight)
			return
		}
		log.Printf("stats elapsed=%s load_mode=qps target_qps=%.2f ok=%d err=%d dropped=%d avg_latency_us=%d achieved_qps=%.2f inflight=%d avg_inflight=%.2f max_inflight_seen=%d max_inflight=%d final=true", elapsed.Truncate(time.Second), targetLoad, ok, errs, dropped, avgLatMic, achievedQPS, inflight, avgInflight, maxInflightSeen, cfg.maxInflight)
		return
	}

	intervalOK := m.okInterval.Swap(0)
	intervalErr := m.errorInterval.Swap(0)
	intervalDropped := m.droppedInterval.Swap(0)
	intervalCompleted := intervalOK + intervalErr
	intervalLatency := m.latencyIntervalUs.Swap(0)
	intervalAvgLatMic := int64(0)
	if intervalCompleted > 0 {
		intervalAvgLatMic = intervalLatency / intervalCompleted
	}
	actualQPS := float64(intervalCompleted) / float64(cfg.logIntervalSeconds)
	if cfg.loadMode == loadModeConcurrency {
		log.Printf("stats elapsed=%s load_mode=concurrency target_workers=%d actual_qps=%.2f ok=%d err=%d dropped=%d total_ok=%d total_err=%d total_dropped=%d inflight=%d avg_inflight=%.2f max_inflight_seen=%d max_inflight=%d avg_latency_us=%d", elapsed.Truncate(time.Second), targetWorkerCount(targetLoad, cfg), actualQPS, intervalOK, intervalErr, intervalDropped, ok, errs, dropped, inflight, avgInflight, maxInflightSeen, cfg.maxInflight, intervalAvgLatMic)
		return
	}
	log.Printf("stats elapsed=%s load_mode=qps target_qps=%.2f actual_qps=%.2f ok=%d err=%d dropped=%d total_ok=%d total_err=%d total_dropped=%d inflight=%d avg_inflight=%.2f max_inflight_seen=%d max_inflight=%d avg_latency_us=%d", elapsed.Truncate(time.Second), targetLoad, actualQPS, intervalOK, intervalErr, intervalDropped, ok, errs, dropped, inflight, avgInflight, maxInflightSeen, cfg.maxInflight, intervalAvgLatMic)
}
