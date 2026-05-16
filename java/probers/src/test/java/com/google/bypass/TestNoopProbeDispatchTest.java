package com.google.bypass;

import static org.junit.jupiter.api.Assertions.assertTrue;

import java.time.Duration;
import java.util.Arrays;
import java.util.concurrent.CountDownLatch;
import java.util.concurrent.TimeUnit;
import java.util.concurrent.atomic.AtomicInteger;
import org.junit.jupiter.api.Test;

final class TestNoopProbeDispatchTest {
  @Test
  void burstCycleUsesCompressedTestQpsWindow() throws Exception {
    Duration unit = Duration.ofMillis(250);
    long startedNanos = System.nanoTime();
    TestNoopProbe probe =
        new TestNoopProbe(
            /* latencyMillis= */ 0,
            /* errorEvery= */ 0,
            () -> (int) ((System.nanoTime() - startedNanos) / unit.toNanos()),
            /* bucketCount= */ 6);

    ProbeRunner.Handle handle =
        ProbeRunner.start(
            probe,
            new ProbeRunner.Options(
                /* startQps= */ 20.0,
                /* endQps= */ 500.0,
                /* stepQpsPercent= */ 0.0,
                /* qpsStepIntervalSeconds= */ 1,
                /* burstEnabled= */ true,
                /* burstAfterSeconds= */ 2,
                /* qpsCycleEnabled= */ true,
                /* highQpsHoldSeconds= */ 1,
                /* lowQpsHoldSeconds= */ 1,
                /* maxInflight= */ 1000,
                /* logIntervalSeconds= */ 10,
                /* warmupCycles= */ 0,
                unit),
            p -> {
              p.probe();
              return true;
            });
    Thread.sleep(1220);
    handle.stop();

    long[] counts = probe.getBucketCounts();
    long lowBefore = counts[0] + counts[1];
    long high = counts[2];
    long transitionAfterReset = counts[3];
    long lowAfter = counts[4];
    assertTrue(
        lowBefore >= 10 && lowBefore <= 50,
        "low-before bucket count="
            + lowBefore
            + " want roughly 40 across first two buckets; buckets="
            + Arrays.toString(counts));
    assertTrue(
        high >= 80 && high >= lowBefore * 6,
        "high bucket count="
            + high
            + " want burst much higher than low-before="
            + lowBefore
            + "; buckets="
            + Arrays.toString(counts));
    assertTrue(
        lowAfter >= 2 && lowAfter * 8 <= high,
        "low-after bucket count="
            + lowAfter
            + " want settled reset much lower than high="
            + high
            + "; transition-after-reset="
            + transitionAfterReset
            + "; buckets="
            + Arrays.toString(counts));
  }

  @Test
  void concurrencyModeStartsMaxInflightWorkers() throws Exception {
    int maxInflight = 4;
    CountDownLatch sawAllWorkersActive = new CountDownLatch(1);
    AtomicInteger active = new AtomicInteger();
    AtomicInteger maxSeen = new AtomicInteger();

    ProbeRunner.Handle handle =
        ProbeRunner.start(
            new TestNoopProbe(),
            new ProbeRunner.Options(
                ProbeRunner.LoadMode.CONCURRENCY,
                /* startQps= */ maxInflight,
                /* endQps= */ 0.0,
                /* stepQpsPercent= */ 0.0,
                /* qpsStepIntervalSeconds= */ 1,
                /* burstEnabled= */ false,
                /* burstAfterSeconds= */ 1,
                /* qpsCycleEnabled= */ false,
                /* highQpsHoldSeconds= */ 0,
                /* lowQpsHoldSeconds= */ 0,
                maxInflight,
                /* logIntervalSeconds= */ 10,
                /* warmupCycles= */ 0,
                Duration.ofMillis(250)),
            p -> {
              int now = active.incrementAndGet();
              maxSeen.accumulateAndGet(now, Math::max);
              if (now == maxInflight) {
                sawAllWorkersActive.countDown();
              }
              try {
                Thread.sleep(100);
                p.probe();
                return true;
              } catch (InterruptedException e) {
                Thread.currentThread().interrupt();
                return false;
              } finally {
                active.decrementAndGet();
              }
            });

    try {
      assertTrue(
          sawAllWorkersActive.await(2, TimeUnit.SECONDS),
          "max active workers=" + maxSeen.get() + " want " + maxInflight);
    } finally {
      handle.stop();
    }
  }
}
