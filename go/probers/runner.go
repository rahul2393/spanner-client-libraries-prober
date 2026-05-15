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
}

func run(ctx context.Context, cfg config, p probe, otelState *otelRuntime) {
	statsTicker := time.NewTicker(time.Duration(cfg.logIntervalSeconds) * time.Second)
	defer statsTicker.Stop()

	targetQPS := newTargetQPS(cfg.startQPS)
	startQPSController(ctx, cfg, targetQPS)

	sem := make(chan struct{}, cfg.maxInflight)
	m := &metrics{}
	start := time.Now()

	for {
		delay := dispatchDelay(targetQPS.Load())
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			log.Printf("stopping (%v)", ctx.Err())
			printSummary(m, time.Since(start), targetQPS.Load(), len(sem), cfg, true)
			return
		case <-statsTicker.C:
			if !timer.Stop() {
				<-timer.C
			}
			printSummary(m, time.Since(start), targetQPS.Load(), len(sem), cfg, false)
		case <-timer.C:
			select {
			case sem <- struct{}{}:
				go func() {
					defer func() { <-sem }()

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
				}()
			default:
				m.droppedCount.Add(1)
				m.droppedInterval.Add(1)
			}
		}
	}
}

type targetQPSState struct {
	value atomic.Uint64
}

func newTargetQPS(qps float64) *targetQPSState {
	s := &targetQPSState{}
	s.Store(qps)
	return s
}

func (s *targetQPSState) Load() float64 {
	return math.Float64frombits(s.value.Load())
}

func (s *targetQPSState) Store(qps float64) {
	s.value.Store(math.Float64bits(qps))
}

func dispatchDelay(qps float64) time.Duration {
	if qps <= 0 {
		return time.Second
	}
	delay := time.Duration(float64(time.Second) / qps)
	if delay <= 0 {
		return time.Nanosecond
	}
	return delay
}

func startQPSController(ctx context.Context, cfg config, targetQPS *targetQPSState) {
	if cfg.burstEnabled {
		go func() {
			timer := time.NewTimer(time.Duration(cfg.burstAfterSeconds) * time.Second)
			defer timer.Stop()
			select {
			case <-ctx.Done():
				return
			case <-timer.C:
				if cfg.endQPS > 0 {
					targetQPS.Store(cfg.endQPS)
					log.Printf("burst_qps target_qps=%.2f", cfg.endQPS)
				}
				startQPSRamp(ctx, cfg, targetQPS)
			}
		}()
		return
	}
	startQPSRamp(ctx, cfg, targetQPS)
}

func startQPSRamp(ctx context.Context, cfg config, targetQPS *targetQPSState) {
	if cfg.stepQPSPercent <= 0 {
		return
	}
	go func() {
		ticker := time.NewTicker(time.Duration(cfg.qpsStepInterval) * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				current := targetQPS.Load()
				if cfg.endQPS > 0 && current >= cfg.endQPS {
					continue
				}
				next := current * (1.0 + cfg.stepQPSPercent/100.0)
				if cfg.endQPS > 0 && next > cfg.endQPS {
					next = cfg.endQPS
				}
				targetQPS.Store(next)
				log.Printf("step_qps old_qps=%.2f new_qps=%.2f", current, next)
			}
		}
	}()
}

func warmup(ctx context.Context, p probe, cycles int) error {
	for i := 0; i < cycles; i++ {
		if err := p.Probe(ctx); err != nil {
			return err
		}
	}
	return nil
}

func printSummary(m *metrics, elapsed time.Duration, targetQPS float64, inflight int, cfg config, final bool) {
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
	if final {
		log.Printf("stats elapsed=%s target_qps=%.2f ok=%d err=%d dropped=%d avg_latency_us=%d achieved_qps=%.2f inflight=%d max_inflight=%d final=true", elapsed.Truncate(time.Second), targetQPS, ok, errs, dropped, avgLatMic, achievedQPS, inflight, cfg.maxInflight)
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
	log.Printf("stats elapsed=%s target_qps=%.2f actual_qps=%.2f ok=%d err=%d dropped=%d total_ok=%d total_err=%d total_dropped=%d inflight=%d max_inflight=%d avg_latency_us=%d", elapsed.Truncate(time.Second), targetQPS, actualQPS, intervalOK, intervalErr, intervalDropped, ok, errs, dropped, inflight, cfg.maxInflight, intervalAvgLatMic)
}
