package com.google.bypass;

import static org.junit.jupiter.api.Assertions.assertDoesNotThrow;
import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertThrows;
import static org.junit.jupiter.api.Assertions.assertTrue;

import org.junit.jupiter.api.Test;

final class QpsControllerTest {
  @Test
  void stepUpQpsClampsAtEnd() {
    assertEquals(150.0, QpsController.stepUpQps(100.0, 500.0, 50.0), 0.0001);
    assertEquals(500.0, QpsController.stepUpQps(400.0, 500.0, 50.0), 0.0001);
    assertEquals(600.0, QpsController.stepUpQps(400.0, 0.0, 50.0), 0.0001);
  }

  @Test
  void stepDownQpsClampsAtStart() {
    assertEquals(500.0, QpsController.stepDownQps(1000.0, 200.0, 50.0), 0.0001);
    assertEquals(200.0, QpsController.stepDownQps(300.0, 200.0, 50.0), 0.0001);
    assertEquals(200.0, QpsController.stepDownQps(1000.0, 200.0, 100.0), 0.0001);
  }

  @Test
  void rampCycleQpsSequenceScalesUpAndDown() {
    double start = 200.0;
    double end = 1000.0;
    double percent = 50.0;
    double qps = start;

    qps = QpsController.stepUpQps(qps, end, percent);
    assertEquals(300.0, qps, 0.0001);
    qps = QpsController.stepUpQps(qps, end, percent);
    assertEquals(450.0, qps, 0.0001);
    while (qps < end) {
      qps = QpsController.stepUpQps(qps, end, percent);
    }
    assertEquals(end, qps, 0.0001);

    qps = QpsController.stepDownQps(qps, start, percent);
    assertEquals(500.0, qps, 0.0001);
    qps = QpsController.stepDownQps(qps, start, percent);
    assertEquals(250.0, qps, 0.0001);
    qps = QpsController.stepDownQps(qps, start, percent);
    assertEquals(start, qps, 0.0001);
  }

  @Test
  void validateQpsKnobsAcceptsBurstCycleWithoutStepPercent() {
    assertDoesNotThrow(
        () ->
            QpsController.validateKnobs(
                100.0,
                1000.0,
                0.0,
                true,
                0,
                true,
                1,
                0));
  }

  @Test
  void validateQpsKnobsRejectsCycleWhenEndNotAboveStart() {
    IllegalArgumentException err =
        assertThrows(
            IllegalArgumentException.class,
            () ->
                QpsController.validateKnobs(
                    100.0,
                    100.0,
                    50.0,
                    false,
                    0,
                    true,
                    0,
                    1));
    assertTrue(err.getMessage().contains("END_LOAD must be greater"));
  }

  @Test
  void validateQpsKnobsRejectsNonBurstCycleWithoutStepPercent() {
    IllegalArgumentException err =
        assertThrows(
            IllegalArgumentException.class,
            () ->
                QpsController.validateKnobs(
                    100.0,
                    1000.0,
                    0.0,
                    false,
                    0,
                    true,
                    0,
                    1));
    assertTrue(err.getMessage().contains("STEP_LOAD_PERCENT"));
  }

  @Test
  void validateQpsKnobsRejectsNegativeHoldSeconds() {
    IllegalArgumentException highHoldErr =
        assertThrows(
            IllegalArgumentException.class,
            () ->
                QpsController.validateKnobs(
                    100.0,
                    1000.0,
                    50.0,
                    true,
                    0,
                    true,
                    -1,
                    0));
    assertTrue(highHoldErr.getMessage().contains("HIGH_LOAD_HOLD_SECONDS"));

    IllegalArgumentException lowHoldErr =
        assertThrows(
            IllegalArgumentException.class,
            () ->
                QpsController.validateKnobs(
                    100.0,
                    1000.0,
                    50.0,
                    false,
                    0,
                    true,
                    0,
                    -1));
    assertTrue(lowHoldErr.getMessage().contains("LOW_LOAD_HOLD_SECONDS"));
  }

  @Test
  void validateQpsKnobsRejectsBurstCycleWithZeroHighHold() {
    IllegalArgumentException err =
        assertThrows(
            IllegalArgumentException.class,
            () ->
                QpsController.validateKnobs(
                    100.0,
                    1000.0,
                    0.0,
                    true,
                    0,
                    true,
                    0,
                    0));
    assertTrue(err.getMessage().contains("HIGH_LOAD_HOLD_SECONDS"));
  }

  @Test
  void validateQpsKnobsRejectsNonBurstCycleWithZeroLowHold() {
    IllegalArgumentException err =
        assertThrows(
            IllegalArgumentException.class,
            () ->
                QpsController.validateKnobs(
                    100.0,
                    1000.0,
                    50.0,
                    false,
                    0,
                    true,
                    0,
                    0));
    assertTrue(err.getMessage().contains("LOW_LOAD_HOLD_SECONDS"));
  }
}
