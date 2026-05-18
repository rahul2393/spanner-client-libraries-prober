package com.google.bypass;

import java.time.Duration;
import java.util.concurrent.ExecutorService;
import java.util.concurrent.Executors;
import java.util.concurrent.RejectedExecutionException;
import java.util.concurrent.ScheduledExecutorService;
import java.util.concurrent.Semaphore;
import java.util.concurrent.SynchronousQueue;
import java.util.concurrent.ThreadPoolExecutor;
import java.util.concurrent.TimeUnit;
import java.util.concurrent.atomic.AtomicLong;
import java.util.concurrent.atomic.AtomicReference;
import java.util.concurrent.locks.LockSupport;
import java.util.logging.Logger;

final class ProbeRunner {
  private static final Logger logger = Logger.getLogger(ProbeRunner.class.getName());

  @FunctionalInterface
  interface ProbeInvoker {
    boolean invoke(Probe probe);
  }

  enum LoadMode {
    QPS,
    CONCURRENCY;

    static LoadMode parse(String value) {
      if (value == null || value.trim().isEmpty()) {
        return QPS;
      }
      String normalized = value.trim().toUpperCase().replace('-', '_');
      for (LoadMode mode : values()) {
        if (mode.name().equals(normalized)) {
          return mode;
        }
      }
      throw new IllegalArgumentException(
          "LOAD_MODE must be one of qps, concurrency. Current value: " + value);
    }
  }

  static final class Options {
    final LoadMode loadMode;
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
      this(
          LoadMode.QPS,
          startQps,
          endQps,
          stepQpsPercent,
          qpsStepIntervalSeconds,
          burstEnabled,
          burstAfterSeconds,
          qpsCycleEnabled,
          highQpsHoldSeconds,
          lowQpsHoldSeconds,
          maxInflight,
          logIntervalSeconds,
          warmupCycles,
          timeUnit);
    }

    Options(
        LoadMode loadMode,
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
      this.loadMode = loadMode == null ? LoadMode.QPS : loadMode;
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
      if (dispatcher != null) {
        dispatcher.interrupt();
        dispatcher.join(1000);
      }
      controlScheduler.shutdownNow();
      if (dispatcher == null) {
        executor.shutdownNow();
      } else {
        executor.shutdown();
      }
      executor.awaitTermination(1, TimeUnit.SECONDS);
    }
  }

  static Handle start(Probe probe, Options options, ProbeInvoker invoker) {
    warmup(probe, options.warmupCycles);

    ExecutorService executor =
        options.loadMode == LoadMode.CONCURRENCY
            ? Executors.newFixedThreadPool(options.maxInflight)
            : createQpsExecutor(options);
    ScheduledExecutorService controlScheduler = Executors.newScheduledThreadPool(2);
    AtomicReference<Double> targetQps = new AtomicReference<>(options.startQps);
    RunStats stats = new RunStats();

    controlScheduler.scheduleAtFixedRate(
        () -> logStats(targetQps, options, stats),
        delayNanos(options.logIntervalSeconds, options),
        delayNanos(options.logIntervalSeconds, options),
        TimeUnit.NANOSECONDS);

    scheduleQpsControl(controlScheduler, targetQps, options);

    if (options.loadMode == LoadMode.CONCURRENCY) {
      startConcurrencyWorkers(executor, probe, options, invoker, stats, targetQps);
      return new Handle(executor, controlScheduler, null);
    }

    Thread dispatcher = startQpsDispatcher(executor, probe, options, invoker, stats, targetQps);
    return new Handle(executor, controlScheduler, dispatcher);
  }

  private static ExecutorService createQpsExecutor(Options options) {
    // Do not use newFixedThreadPool(maxInflight): maxInflight becomes the core size, so
    // early probe dispatches create one worker per request until that limit, even when
    // existing workers are idle. Reusing idle workers keeps burst timing from being skewed
    // by thread-start lag; the semaphore still caps in-flight probes.
    return new ThreadPoolExecutor(
        0, options.maxInflight, 60L, TimeUnit.SECONDS, new SynchronousQueue<>());
  }

  private static void startConcurrencyWorkers(
      ExecutorService executor,
      Probe probe,
      Options options,
      ProbeInvoker invoker,
      RunStats stats,
      AtomicReference<Double> targetWorkers) {
    for (int i = 0; i < options.maxInflight; i++) {
      int workerNumber = i + 1;
      executor.execute(
          () -> {
            while (!Thread.currentThread().isInterrupted()) {
              if (workerNumber <= targetWorkerCount(targetWorkers.get(), options)) {
                runProbe(probe, invoker, stats);
              } else {
                sleepQuietly(100, TimeUnit.MILLISECONDS);
              }
            }
          });
    }
  }

  private static long targetWorkerCount(double target, Options options) {
    return Math.max(0L, Math.min(options.maxInflight, Math.round(target)));
  }

  private static Thread startQpsDispatcher(
      ExecutorService executor,
      Probe probe,
      Options options,
      ProbeInvoker invoker,
      RunStats stats,
      AtomicReference<Double> targetQps) {
    Semaphore permits = new Semaphore(options.maxInflight);
    Thread dispatcher =
        new Thread(
            () -> {
              long lastNanos = System.nanoTime();
              double tokens = 0.0d;
              long tickNanos = dispatchTickNanos(options);
              while (!Thread.currentThread().isInterrupted()) {
                LockSupport.parkNanos(tickNanos);
                if (Thread.currentThread().isInterrupted()) {
                  return;
                }
                long now = System.nanoTime();
                long elapsed = Math.max(0L, now - lastNanos);
                lastNanos = now;
                tokens += targetQps.get() * ((double) elapsed / options.timeUnit.toNanos());
                tokens = Math.min(tokens, options.maxInflight);
                int toDispatch = (int) tokens;
                if (toDispatch == 0) {
                  continue;
                }
                tokens -= toDispatch;
                for (int i = 0; i < toDispatch; i++) {
                  if (!permits.tryAcquire()) {
                    stats.recordDropped();
                    continue;
                  }
                  try {
                    executor.execute(
                        () -> {
                          try {
                            runProbe(probe, invoker, stats);
                          } finally {
                            permits.release();
                          }
                        });
                  } catch (RejectedExecutionException e) {
                    permits.release();
                    if (!Thread.currentThread().isInterrupted()) {
                      stats.recordDropped();
                    }
                  }
                }
              }
            },
            "probe-dispatcher");
    dispatcher.start();
    return dispatcher;
  }

  private static long dispatchTickNanos(Options options) {
    return Math.max(TimeUnit.MILLISECONDS.toNanos(1), options.timeUnit.toNanos() / 100L);
  }

  private static void runProbe(Probe probe, ProbeInvoker invoker, RunStats stats) {
    long start = System.nanoTime();
    stats.recordStarted();
    try {
      if (invoker.invoke(probe)) {
        stats.recordSuccess();
      } else {
        stats.recordError();
      }
    } finally {
      stats.recordFinished((System.nanoTime() - start) / 1000L);
    }
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
              logger.info(
                  String.format(
                      "burst %s from %.2f to %.2f",
                      loadUnit(options), old, options.endQps));
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
                  "burst %s from %.2f to %.2f hold_seconds=%d",
                  loadUnit(options), old, options.endQps, options.highQpsHoldSeconds));
          controlScheduler.schedule(
              () -> {
                double previous = targetQps.getAndSet(options.startQps);
                logger.info(
                    String.format(
                        "reset %s from %.2f to %.2f hold_seconds=%d",
                        loadUnit(options), previous, options.startQps, options.burstAfterSeconds));
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
            logger.info(
                String.format("step up %s from %.2f to %.2f", loadUnit(options), current, next));
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
                    "step down %s from %.2f to %.2f hold_seconds=%d",
                    loadUnit(options), current, next, options.lowQpsHoldSeconds));
            scheduleRampQpsCycle(
                controlScheduler,
                targetQps,
                options,
                /* direction= */ 1,
                options.lowQpsHoldSeconds);
            return;
          }
          logger.info(
              String.format("step down %s from %.2f to %.2f", loadUnit(options), current, next));
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
    logger.info(String.format("step %s from %.2f to %.2f", loadUnit(options), current, next));
  }

  private static void logStats(
      AtomicReference<Double> targetQps,
      Options options,
      RunStats stats) {
    long ok = stats.successInterval.getAndSet(0);
    long errors = stats.errorInterval.getAndSet(0);
    long dropped = stats.droppedInterval.getAndSet(0);
    long completed = ok + errors;
    long avgLatencyMicros =
        completed == 0 ? 0 : stats.totalLatencyMicros.getAndSet(0) / completed;
    double intervalSeconds = delayNanos(options.logIntervalSeconds, options) / 1_000_000_000.0;
    double actualQps = intervalSeconds <= 0 ? 0 : (ok + errors) / intervalSeconds;
    long currentInflight = stats.currentInflight.get();
    long maxInflightSeen = stats.maxInflightInterval.getAndSet(currentInflight);
    long inflightSamples = stats.inflightSampleCount.getAndSet(1);
    long inflightSampleSum = stats.inflightSampleSum.getAndSet(currentInflight);
    double avgInflight =
        inflightSamples == 0 ? currentInflight : (double) inflightSampleSum / inflightSamples;
    logger.info(
        options.loadMode == LoadMode.CONCURRENCY
            ? String.format(
                "stats load_mode=concurrency target_workers=%d actual_qps=%.2f ok=%d err=%d dropped=%d total_ok=%d total_err=%d total_dropped=%d inflight=%d avg_inflight=%.2f max_inflight_seen=%d max_inflight=%d avg_latency_us=%d",
                targetWorkerCount(targetQps.get(), options),
                actualQps,
                ok,
                errors,
                dropped,
                stats.successTotal.get(),
                stats.errorTotal.get(),
                stats.droppedTotal.get(),
                currentInflight,
                avgInflight,
                maxInflightSeen,
                options.maxInflight,
                avgLatencyMicros)
            : String.format(
                "stats load_mode=qps target_qps=%.2f actual_qps=%.2f ok=%d err=%d dropped=%d total_ok=%d total_err=%d total_dropped=%d inflight=%d avg_inflight=%.2f max_inflight_seen=%d max_inflight=%d avg_latency_us=%d",
                targetQps.get(),
                actualQps,
                ok,
                errors,
                dropped,
                stats.successTotal.get(),
                stats.errorTotal.get(),
                stats.droppedTotal.get(),
                currentInflight,
                avgInflight,
                maxInflightSeen,
                options.maxInflight,
                avgLatencyMicros));
  }

  private static long delayNanos(int units, Options options) {
    return Math.max(1L, units * options.timeUnit.toNanos());
  }

  private static String loadUnit(Options options) {
    return options.loadMode == LoadMode.CONCURRENCY ? "workers" : "qps";
  }

  private static void sleepQuietly(long duration, TimeUnit unit) {
    try {
      unit.sleep(duration);
    } catch (InterruptedException e) {
      Thread.currentThread().interrupt();
    }
  }

  private static final class RunStats {
    private final AtomicLong successTotal = new AtomicLong();
    private final AtomicLong errorTotal = new AtomicLong();
    private final AtomicLong droppedTotal = new AtomicLong();
    private final AtomicLong successInterval = new AtomicLong();
    private final AtomicLong errorInterval = new AtomicLong();
    private final AtomicLong droppedInterval = new AtomicLong();
    private final AtomicLong totalLatencyMicros = new AtomicLong();
    private final AtomicLong currentInflight = new AtomicLong();
    private final AtomicLong maxInflightInterval = new AtomicLong();
    private final AtomicLong inflightSampleSum = new AtomicLong();
    private final AtomicLong inflightSampleCount = new AtomicLong();

    private void recordStarted() {
      long inflight = currentInflight.incrementAndGet();
      maxInflightInterval.accumulateAndGet(inflight, Math::max);
      recordInflightSample(inflight);
    }

    private void recordFinished(long latencyMicros) {
      totalLatencyMicros.addAndGet(latencyMicros);
      recordInflightSample(currentInflight.decrementAndGet());
    }

    private void recordSuccess() {
      successTotal.incrementAndGet();
      successInterval.incrementAndGet();
    }

    private void recordError() {
      errorTotal.incrementAndGet();
      errorInterval.incrementAndGet();
    }

    private void recordDropped() {
      droppedTotal.incrementAndGet();
      droppedInterval.incrementAndGet();
    }

    private void recordInflightSample(long inflight) {
      inflightSampleSum.addAndGet(inflight);
      inflightSampleCount.incrementAndGet();
    }
  }

  private ProbeRunner() {}
}
