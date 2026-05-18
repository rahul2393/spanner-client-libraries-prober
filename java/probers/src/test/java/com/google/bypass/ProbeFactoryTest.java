package com.google.bypass;

import static org.junit.jupiter.api.Assertions.assertDoesNotThrow;
import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertInstanceOf;
import static org.junit.jupiter.api.Assertions.assertThrows;
import static org.junit.jupiter.api.Assertions.assertTrue;

import org.junit.jupiter.api.Test;

final class ProbeFactoryTest {
  @Test
  void testNoopDoesNotRequireSpannerClient() {
    Probe probe = createTestNoopProbe();

    TestNoopProbe noop = assertInstanceOf(TestNoopProbe.class, probe);
    assertEquals("test_noop", noop.getName());
    assertDoesNotThrow(noop::probe);
    assertEquals(1, noop.getCount());
  }

  @Test
  void testNoopInjectedErrors() {
    TestNoopProbe probe = new TestNoopProbe(/* latencyMillis= */ 0, /* errorEvery= */ 2);

    assertDoesNotThrow(probe::probe);
    RuntimeException err = assertThrows(RuntimeException.class, probe::probe);

    assertEquals(2, probe.getCount());
    assertTrue(err.getMessage().contains("injected error"));
  }

  @Test
  void testNoopProbeAcceptsSteadyAndCycleQpsKnobs() {
    assertValidNoopConfig("no dcp steady", false, 100.0, 0.0, 0.0, false, false, 0, 0);
    assertValidNoopConfig("dcp steady", true, 100.0, 0.0, 0.0, false, false, 0, 0);
    assertValidNoopConfig("dcp ramp cycle", true, 100.0, 1000.0, 50.0, false, true, 0, 1);
    assertValidNoopConfig("dcp burst cycle", true, 100.0, 1000.0, 0.0, true, true, 1, 0);
    assertValidNoopConfig("no dcp burst cycle", false, 100.0, 1000.0, 0.0, true, true, 1, 0);
  }

  private static void assertValidNoopConfig(
      String name,
      boolean enableDynamicChannelPool,
      double startQps,
      double endQps,
      double stepQpsPercent,
      boolean burstEnabled,
      boolean qpsCycleEnabled,
      int highQpsHoldSeconds,
      int lowQpsHoldSeconds) {
    assertDoesNotThrow(
        () ->
            QpsController.validateKnobs(
                startQps,
                endQps,
                stepQpsPercent,
                burstEnabled,
                /* burstAfterSeconds= */ 0,
                qpsCycleEnabled,
                highQpsHoldSeconds,
                lowQpsHoldSeconds),
        name);
    // test_noop intentionally does not build a Spanner client, so DCP/no-DCP only changes
    // the test case shape. The probe path should remain valid in both modes.
    TestNoopProbe probe =
        assertInstanceOf(
            TestNoopProbe.class, createTestNoopProbe(), name + " dcp=" + enableDynamicChannelPool);
    assertDoesNotThrow(probe::probe, name);
    assertEquals(1, probe.getCount(), name);
  }

  private static Probe createTestNoopProbe() {
    return ProbeFactory.create(
        /* client= */ null,
        "test_noop",
        /* numRows= */ 1,
        /* payloadSize= */ 1,
        /* maxStalenessSeconds= */ 1,
        "usertable",
        "",
        "1",
        /* ycsbZeroPadding= */ 20,
        /* fixedKey= */ -1);
  }
}
