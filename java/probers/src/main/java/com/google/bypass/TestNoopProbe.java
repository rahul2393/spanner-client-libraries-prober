package com.google.bypass;

import java.util.concurrent.atomic.AtomicLong;
import java.util.concurrent.atomic.AtomicLongArray;
import java.util.function.IntSupplier;

// TestNoopProbe is an internal test workload. It exercises the prober dispatcher and
// QPS controller without creating a Spanner client or issuing RPCs.
final class TestNoopProbe implements Probe {
  private final AtomicLong count = new AtomicLong();
  private final long latencyMillis;
  private final long errorEvery;
  private final IntSupplier bucketSupplier;
  private final AtomicLongArray buckets;

  TestNoopProbe() {
    this(0, 0);
  }

  TestNoopProbe(long latencyMillis, long errorEvery) {
    this(latencyMillis, errorEvery, null, 0);
  }

  TestNoopProbe(
      long latencyMillis, long errorEvery, IntSupplier bucketSupplier, int bucketCount) {
    if (latencyMillis < 0) {
      throw new IllegalArgumentException("latencyMillis must be >= 0");
    }
    if (errorEvery < 0) {
      throw new IllegalArgumentException("errorEvery must be >= 0");
    }
    if (bucketCount < 0) {
      throw new IllegalArgumentException("bucketCount must be >= 0");
    }
    this.latencyMillis = latencyMillis;
    this.errorEvery = errorEvery;
    this.bucketSupplier = bucketSupplier;
    this.buckets = bucketCount == 0 ? null : new AtomicLongArray(bucketCount);
  }

  @Override
  public String getName() {
    return "test_noop";
  }

  @Override
  public void probe() {
    if (latencyMillis > 0) {
      try {
        Thread.sleep(latencyMillis);
      } catch (InterruptedException e) {
        Thread.currentThread().interrupt();
        throw new RuntimeException("test_noop interrupted", e);
      }
    }
    long n = count.incrementAndGet();
    recordBucket();
    if (errorEvery > 0 && n % errorEvery == 0) {
      throw new RuntimeException("test_noop injected error at call " + n);
    }
  }

  long getCount() {
    return count.get();
  }

  long[] getBucketCounts() {
    if (buckets == null) {
      return new long[0];
    }
    long[] counts = new long[buckets.length()];
    for (int i = 0; i < buckets.length(); i++) {
      counts[i] = buckets.get(i);
    }
    return counts;
  }

  private void recordBucket() {
    if (bucketSupplier == null || buckets == null) {
      return;
    }
    int bucket = bucketSupplier.getAsInt();
    if (bucket < 0 || bucket >= buckets.length()) {
      return;
    }
    buckets.incrementAndGet(bucket);
  }
}
