package main

import (
	"context"
	"fmt"
	"sync/atomic"
	"time"
)

// noopProbe is an internal test probe. It exercises the prober dispatcher and
// QPS controller without creating a Spanner client or issuing RPCs.
type noopProbe struct {
	count      atomic.Int64
	latency    time.Duration
	errorEvery int64
	bucketFn   func() int
	buckets    []atomic.Int64
}

func (p *noopProbe) Name() string { return "test_noop" }

func (p *noopProbe) Probe(ctx context.Context) error {
	if p.latency > 0 {
		timer := time.NewTimer(p.latency)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
	n := p.count.Add(1)
	p.recordBucket()
	if p.errorEvery > 0 && n%p.errorEvery == 0 {
		return fmt.Errorf("test_noop injected error at call %d", n)
	}
	return nil
}

func (p *noopProbe) Count() int64 { return p.count.Load() }

func (p *noopProbe) recordBucket() {
	if p.bucketFn == nil || len(p.buckets) == 0 {
		return
	}
	idx := p.bucketFn()
	if idx < 0 || idx >= len(p.buckets) {
		return
	}
	p.buckets[idx].Add(1)
}

func (p *noopProbe) BucketCounts() []int64 {
	counts := make([]int64, len(p.buckets))
	for i := range p.buckets {
		counts[i] = p.buckets[i].Load()
	}
	return counts
}
