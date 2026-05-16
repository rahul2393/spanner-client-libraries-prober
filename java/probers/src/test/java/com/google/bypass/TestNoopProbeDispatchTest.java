package com.google.bypass;

import static org.junit.jupiter.api.Assertions.assertTrue;

import java.time.Duration;
import java.util.Arrays;
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
            /* bucketCount= */ 4);

    ProbeRunner.Handle handle =
        ProbeRunner.start(
            probe,
            new ProbeRunner.Options(
                /* startQps= */ 10.0,
                /* endQps= */ 500.0,
                /* stepQpsPercent= */ 0.0,
                /* qpsStepIntervalSeconds= */ 1,
                /* burstEnabled= */ true,
                /* burstAfterSeconds= */ 1,
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
    Thread.sleep(740);
    handle.stop();

    long[] counts = probe.getBucketCounts();
    long lowBefore = counts[0];
    long high = counts[1];
    long lowAfter = counts[2];
    assertTrue(
        lowBefore >= 5 && lowBefore <= 25,
        "low-before bucket count="
            + lowBefore
            + " want roughly 10; buckets="
            + Arrays.toString(counts));
    assertTrue(
        high >= 100 && high >= lowBefore * 8,
        "high bucket count="
            + high
            + " want burst much higher than low-before="
            + lowBefore
            + "; buckets="
            + Arrays.toString(counts));
    assertTrue(
        lowAfter >= 5 && lowAfter * 4 <= high,
        "low-after bucket count="
            + lowAfter
            + " want reset much lower than high="
            + high
            + "; buckets="
            + Arrays.toString(counts));
  }
}
