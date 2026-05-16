package com.google.bypass;

import java.time.Duration;
import java.util.concurrent.ExecutorService;
import java.util.concurrent.Executors;
import java.util.concurrent.ScheduledExecutorService;
import java.util.concurrent.Semaphore;
import java.util.concurrent.TimeUnit;
import java.util.concurrent.atomic.AtomicLong;
import java.util.concurrent.atomic.AtomicReference;
import java.util.logging.Logger;

final class ProbeRunner {
  private static final Logger logger = Logger.getLogger(ProbeRunner.class.getName());

  @FunctionalInterface
  interface ProbeInvoker {
    boolean invoke(Probe probe);
  }

  static final class Options {
    final double startQps;
    final double endQps;
    final double stepQpsPercent;
    final int qpsStepIntervalSeconds;
    final boolean burstEnabled;
    final int burstAfterSeconds;
    final boolean qpsCycleEnabled;
    final int highQpsHoldSeconds;
    final int lowQpsHoldSeconds;
    final int maxInflight;
    final int logIntervalSeconds;
    final int warmupCycles;
    final Duration timeUnit;

    Options(
        double startQps,
        double endQps,
        double stepQpsPercent,
        int qpsStepIntervalSeconds,
        boolean burstEnabled,
        int burstAfterSeconds,
        boolean qpsCycleEnabled,
        int highQpsHoldSeconds,
        int lowQpsHoldSeconds,
        int maxInflight,
        int logIntervalSeconds,
        int warmupCycles,
        Duration timeUnit) {
      this.startQps = startQps;
      this.endQps = endQps;
      this.stepQpsPercent = stepQpsPercent;
      this.qpsStepIntervalSeconds = qpsStepIntervalSeconds;
      this.burstEnabled = burstEnabled;
      this.burstAfterSeconds = burstAfterSeconds;
      this.qpsCycleEnabled = qpsCycleEnabled;
      this.highQpsHoldSeconds = highQpsHoldSeconds;
      this.lowQpsHoldSeconds = lowQpsHoldSeconds;
      this.maxInflight = maxInflight;
      this.logIntervalSeconds = logIntervalSeconds;
      this.warmupCycles = warmupCycles;
      this.timeUnit =
          timeUnit == null || timeUnit.isZero() || timeUnit.isNegative()
              ? Duration.ofSeconds(1)
              : timeUnit;
    }
  }

  static final class Handle {
    private final ExecutorService executor;
    private final ScheduledExecutorService controlScheduler;
    private final Thread dispatcher;

    Handle(ExecutorService executor, ScheduledExecutorService controlScheduler, Thread dispatcher) {
      this.executor = executor;
      this.controlScheduler = controlScheduler;
      this.dispatcher = dispatcher;
    }

    void stop() throws InterruptedException {
      dispatcher.interrupt();
      dispatcher.join(1000);
      controlScheduler.shutdownNow();
      executor.shutdown();
      executor.awaitTermination(1, TimeUnit.SECONDS);
    }
  }

  static Handle start(Probe probe, Options options, ProbeInvoker invoker) {
    warmup(probe, options.warmupCycles);

    ExecutorService executor = Executors.newFixedThreadPool(options.maxInflight);
    ScheduledExecutorService controlScheduler = Executors.newScheduledThreadPool(2);
    Semaphore permits = new Semaphore(options.maxInflight);
    AtomicReference<Double> targetQps = new AtomicReference<>(options.startQps);
    AtomicLong successTotal = new AtomicLong();
    AtomicLong errorTotal = new AtomicLong();
    AtomicLong droppedTotal = new AtomicLong();
    AtomicLong successInterval = new AtomicLong();
    AtomicLong errorInterval = new AtomicLong();
    AtomicLong droppedInterval = new AtomicLong();
    AtomicLong totalLatencyMicros = new AtomicLong();

    scheduleQpsControl(controlScheduler, targetQps, options);
    controlScheduler.scheduleAtFixedRate(
        () ->
            logStats(
                targetQps,
                permits,
                options,
                successTotal,
                errorTotal,
                droppedTotal,
                successInterval,
                errorInterval,
                droppedInterval,
                totalLatencyMicros),
        delayNanos(options.logIntervalSeconds, options),
        delayNanos(options.logIntervalSeconds, options),
        TimeUnit.NANOSECONDS);

    Thread dispatcher =
        new Thread(
            () -> {
              while (!Thread.currentThread().isInterrupted()) {
                sleepForQps(targetQps.get(), options.timeUnit);
                if (Thread.currentThread().isInterrupted()) {
                  return;
                }
                if (!permits.tryAcquire()) {
                  droppedTotal.incrementAndGet();
                  droppedInterval.incrementAndGet();
                  continue;
                }
                executor.execute(
                    () -> {
                      long start = System.nanoTime();
                      try {
                        boolean success = invoker.invoke(probe);
                        if (success) {
                          successTotal.incrementAndGet();
                          successInterval.incrementAndGet();
                        } else {
                          errorTotal.incrementAndGet();
                          errorInterval.incrementAndGet();
                        }
                      } finally {
                        totalLatencyMicros.addAndGet((System.nanoTime() - start) / 1000L);
                        permits.release();
                      }
                    });
              }
            },
            "probe-dispatcher");
    dispatcher.start();
    return new Handle(executor, controlScheduler, dispatcher);
  }

  private static void warmup(Probe probe, int warmupCycles) {
    for (int i = 0; i < warmupCycles; i++) {
      probe.probe();
    }
  }

  private static void scheduleQpsControl(
      ScheduledExecutorService controlScheduler,
      AtomicReference<Double> targetQps,
      Options options) {
    if (options.burstEnabled && options.qpsCycleEnabled) {
      scheduleBurstQpsCycle(controlScheduler, targetQps, options);
      return;
    }
    if (!options.burstEnabled && options.qpsCycleEnabled) {
      scheduleRampQpsCycle(
          controlScheduler,
          targetQps,
          options,
          /* direction= */ 1,
          options.qpsStepIntervalSeconds);
      return;
    }
    Runnable startStepping =
        () -> {
          if (options.qpsStepIntervalSeconds > 0 && options.stepQpsPercent > 0) {
            controlScheduler.scheduleAtFixedRate(
                () -> stepQps(targetQps, options),
                delayNanos(options.qpsStepIntervalSeconds, options),
                delayNanos(options.qpsStepIntervalSeconds, options),
                TimeUnit.NANOSECONDS);
          }
        };
    if (options.burstEnabled) {
      controlScheduler.schedule(
          () -> {
            if (options.endQps > 0) {
              double old = targetQps.getAndSet(options.endQps);
              logger.info(String.format("burst qps from %.2f to %.2f", old, options.endQps));
            }
            startStepping.run();
          },
          delayNanos(options.burstAfterSeconds, options),
          TimeUnit.NANOSECONDS);
    } else {
      startStepping.run();
    }
  }

  private static void scheduleBurstQpsCycle(
      ScheduledExecutorService controlScheduler,
      AtomicReference<Double> targetQps,
      Options options) {
    controlScheduler.schedule(
        () -> {
          double old = targetQps.getAndSet(options.endQps);
          logger.info(
              String.format(
                  "burst qps from %.2f to %.2f hold_seconds=%d",
                  old, options.endQps, options.highQpsHoldSeconds));
          controlScheduler.schedule(
              () -> {
                double previous = targetQps.getAndSet(options.startQps);
                logger.info(
                    String.format(
                        "reset qps from %.2f to %.2f hold_seconds=%d",
                        previous, options.startQps, options.burstAfterSeconds));
                scheduleBurstQpsCycle(controlScheduler, targetQps, options);
              },
              delayNanos(options.highQpsHoldSeconds, options),
              TimeUnit.NANOSECONDS);
        },
        delayNanos(options.burstAfterSeconds, options),
        TimeUnit.NANOSECONDS);
  }

  private static void scheduleRampQpsCycle(
      ScheduledExecutorService controlScheduler,
      AtomicReference<Double> targetQps,
      Options options,
      int direction,
      int delaySeconds) {
    controlScheduler.schedule(
        () -> {
          double current = targetQps.get();
          if (direction > 0) {
            double next =
                QpsController.stepUpQps(current, options.endQps, options.stepQpsPercent);
            targetQps.set(next);
            logger.info(String.format("step up qps from %.2f to %.2f", current, next));
            scheduleRampQpsCycle(
                controlScheduler,
                targetQps,
                options,
                next == options.endQps ? -1 : 1,
                options.qpsStepIntervalSeconds);
            return;
          }

          double next =
              QpsController.stepDownQps(current, options.startQps, options.stepQpsPercent);
          targetQps.set(next);
          if (next == options.startQps) {
            logger.info(
                String.format(
                    "step down qps from %.2f to %.2f hold_seconds=%d",
                    current, next, options.lowQpsHoldSeconds));
            scheduleRampQpsCycle(
                controlScheduler,
                targetQps,
                options,
                /* direction= */ 1,
                options.lowQpsHoldSeconds);
            return;
          }
          logger.info(String.format("step down qps from %.2f to %.2f", current, next));
          scheduleRampQpsCycle(
              controlScheduler,
              targetQps,
              options,
              /* direction= */ -1,
              options.qpsStepIntervalSeconds);
        },
        delayNanos(delaySeconds, options),
        TimeUnit.NANOSECONDS);
  }

  private static void stepQps(AtomicReference<Double> targetQps, Options options) {
    double current = targetQps.get();
    if (options.endQps > 0 && current >= options.endQps) {
      return;
    }
    double next = QpsController.stepUpQps(current, options.endQps, options.stepQpsPercent);
    targetQps.set(next);
    logger.info(String.format("step qps from %.2f to %.2f", current, next));
  }

  private static void sleepForQps(double qps, Duration unit) {
    long sleepNanos = Math.max(1L, Math.round(unit.toNanos() / Math.max(0.001d, qps)));
    try {
      TimeUnit.NANOSECONDS.sleep(sleepNanos);
    } catch (InterruptedException e) {
      Thread.currentThread().interrupt();
    }
  }

  private static void logStats(
      AtomicReference<Double> targetQps,
      Semaphore permits,
      Options options,
      AtomicLong successTotal,
      AtomicLong errorTotal,
      AtomicLong droppedTotal,
      AtomicLong successInterval,
      AtomicLong errorInterval,
      AtomicLong droppedInterval,
      AtomicLong totalLatencyMicros) {
    long ok = successInterval.getAndSet(0);
    long errors = errorInterval.getAndSet(0);
    long dropped = droppedInterval.getAndSet(0);
    long completed = ok + errors;
    long avgLatencyMicros = completed == 0 ? 0 : totalLatencyMicros.getAndSet(0) / completed;
    double intervalSeconds = delayNanos(options.logIntervalSeconds, options) / 1_000_000_000.0;
    double actualQps = intervalSeconds <= 0 ? 0 : (ok + errors) / intervalSeconds;
    logger.info(
        String.format(
            "stats target_qps=%.2f actual_qps=%.2f ok=%d err=%d dropped=%d total_ok=%d total_err=%d total_dropped=%d inflight=%d max_inflight=%d avg_latency_us=%d",
            targetQps.get(),
            actualQps,
            ok,
            errors,
            dropped,
            successTotal.get(),
            errorTotal.get(),
            droppedTotal.get(),
            options.maxInflight - permits.availablePermits(),
            options.maxInflight,
            avgLatencyMicros));
  }

  private static long delayNanos(int units, Options options) {
    return Math.max(1L, units * options.timeUnit.toNanos());
  }

  private ProbeRunner() {}
}
