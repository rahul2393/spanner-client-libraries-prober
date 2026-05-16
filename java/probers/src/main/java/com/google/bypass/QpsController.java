package com.google.bypass;

/** Pure QPS helper logic used by the prober runtime and unit tests. */
final class QpsController {
  static void validateKnobs(
      double startQps,
      double endQps,
      double stepQpsPercent,
      boolean burstEnabled,
      int burstAfterSeconds,
      boolean qpsCycleEnabled,
      int highQpsHoldSeconds,
      int lowQpsHoldSeconds) {
    if (burstAfterSeconds < 0) {
      throw new IllegalArgumentException(
          "BURST_AFTER_SECONDS must be >= 0. Current value: " + burstAfterSeconds);
    }
    if (highQpsHoldSeconds < 0) {
      throw new IllegalArgumentException(
          "HIGH_QPS_HOLD_SECONDS must be >= 0. Current value: " + highQpsHoldSeconds);
    }
    if (lowQpsHoldSeconds < 0) {
      throw new IllegalArgumentException(
          "LOW_QPS_HOLD_SECONDS must be >= 0. Current value: " + lowQpsHoldSeconds);
    }
    if (qpsCycleEnabled && endQps <= startQps) {
      throw new IllegalArgumentException(
          String.format(
              "END_QPS must be greater than START_QPS/QPS when QPS_CYCLE_ENABLED=true. "
                  + "start=%.2f end=%.2f",
              startQps, endQps));
    }
    if (qpsCycleEnabled && burstEnabled && highQpsHoldSeconds <= 0) {
      throw new IllegalArgumentException(
          "HIGH_QPS_HOLD_SECONDS must be > 0 for burst QPS_CYCLE_ENABLED=true.");
    }
    if (qpsCycleEnabled && !burstEnabled && lowQpsHoldSeconds <= 0) {
      throw new IllegalArgumentException(
          "LOW_QPS_HOLD_SECONDS must be > 0 for non-burst QPS_CYCLE_ENABLED=true.");
    }
    if (qpsCycleEnabled && !burstEnabled && stepQpsPercent <= 0) {
      throw new IllegalArgumentException(
          "STEP_QPS_PERCENT/STEP_QPS must be > 0 for non-burst QPS_CYCLE_ENABLED=true.");
    }
  }

  static double stepUpQps(double current, double end, double percent) {
    double next = current + (current * percent / 100.0);
    if (end > 0) {
      return Math.min(next, end);
    }
    return next;
  }

  static double stepDownQps(double current, double start, double percent) {
    if (percent >= 100) {
      return start;
    }
    return Math.max(current - (current * percent / 100.0), start);
  }

  private QpsController() {}
}
