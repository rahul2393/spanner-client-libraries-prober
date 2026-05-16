# Java Spanner probers

Java prober app for Spanner client library experiments. Same jar can run different operations through `PROBE_TYPE`.

## Build released google-cloud-spanner image

```bash
./java/probers/scripts/build-images.sh release 6.114.0
./java/probers/scripts/build-images.sh release 6.117.0
```

This uses Maven Central artifact `com.google.cloud:google-cloud-spanner:<version>`.

## Build image from unreleased google-cloud-java branch/SHA

Push branch first, then:

```bash
./java/probers/scripts/build-images.sh source <branch-or-sha>
```

Defaults:

```bash
GCJ_REPO_URL=https://github.com/rahul2393/google-cloud-java.git
IMAGE_REPO=us-central1-docker.pkg.dev/span-cloud-testing/irahul-images/irahul-client
```

Override example:

```bash
GCJ_REPO_URL=https://github.com/googleapis/google-cloud-java.git \
  ./java/probers/scripts/build-images.sh source main
```

Push image:

```bash
./java/probers/scripts/build-images.sh release 6.117.0 --push
./java/probers/scripts/build-images.sh source directpath-fixes-all --push
```

## Direct Docker commands

Release:

```bash
docker build \
  -f java/probers/Dockerfile \
  --target runtime-release \
  --build-arg SPANNER_VERSION=6.117.0 \
  -t us-central1-docker.pkg.dev/span-cloud-testing/irahul-images/irahul-client:6.117.0 \
  .
```

Source branch/SHA:

```bash
docker build \
  -f java/probers/Dockerfile \
  --target runtime-source \
  --build-arg GCJ_REPO_URL=https://github.com/rahul2393/google-cloud-java.git \
  --build-arg GCJ_REF=<branch-or-sha> \
  -t us-central1-docker.pkg.dev/span-cloud-testing/irahul-images/irahul-client:<branch-tag> \
  .
```

## Load and DCP knobs

The dispatcher is decoupled from probe execution: it submits work at target QPS, caps in-flight work, and logs dropped submissions when the client is saturated. Each probe operation is synchronous, so `MAX_INFLIGHT` is also the Java worker thread count.

Use these knobs for any `PROBE_TYPE`:

| Env | Default | Meaning |
| --- | --- | --- |
| `QPS` or `START_QPS` | `4` | Initial target QPS |
| `END_QPS` | `0` | Ramp cap. `0` means no cap |
| `STEP_QPS_PERCENT` or `STEP_QPS` | `0` | Increase target QPS by this percent every interval |
| `QPS_STEP_INTERVAL_SECONDS` or `INTERVAL_SECONDS` | `60` | Ramp interval |
| `BURST_ENABLED` or `BURST_MODE` | `false` | Jump to `END_QPS` after `BURST_AFTER_SECONDS` |
| `BURST_AFTER_SECONDS` | `900` | Burst delay |
| `QPS_CYCLE_ENABLED` or `CYCLE_ENABLED` | `false` | Continuously cycle QPS for scale-up/scale-down testing |
| `HIGH_QPS_HOLD_SECONDS` | `300` | In burst cycle mode, hold `END_QPS` for this many seconds |
| `LOW_QPS_HOLD_SECONDS` | `300` | In ramp cycle mode, hold `START_QPS` after step-down for this many seconds |
| `MAX_INFLIGHT` or `PARALLELISM` | `64` | Max active Java worker/probe operations |
| `LOG_INTERVAL_SECONDS` | `10` | Stats log interval |
| `ENABLE_DYNAMIC_CHANNEL_POOL` or `SPANNER_DCP_ENABLED` or `DCP_ENABLED` | `false` | Calls `SpannerOptions.enableDynamicChannelPool()` |
| `DISABLE_DYNAMIC_CHANNEL_POOL` | `false` | Calls `SpannerOptions.disableDynamicChannelPool()`; wins if both are true |

QPS controller modes:

- default ramp: `START_QPS` ramps up to `END_QPS` by `STEP_QPS_PERCENT`, then stays at `END_QPS`;
- burst: `START_QPS` jumps to `END_QPS` once after `BURST_AFTER_SECONDS`;
- ramp cycle: with `QPS_CYCLE_ENABLED=true` and `BURST_ENABLED=false`, QPS ramps up to `END_QPS`, steps down to `START_QPS`, holds `LOW_QPS_HOLD_SECONDS`, then repeats;
- burst cycle: with both `QPS_CYCLE_ENABLED=true` and `BURST_ENABLED=true`, QPS waits `BURST_AFTER_SECONDS`, jumps to `END_QPS`, holds `HIGH_QPS_HOLD_SECONDS`, resets to `START_QPS`, then repeats.

For DCP scale-down validation, keep `LOW_QPS_HOLD_SECONDS` long enough for the Java client scale-down check interval, consecutive low-load checks, and drain idle grace.

DCP stress example:

```yaml
env:
  - name: PROBE_TYPE
    value: "stale_query"
  - name: START_QPS
    value: "100"
  - name: END_QPS
    value: "5000"
  - name: STEP_QPS_PERCENT
    value: "25"
  - name: QPS_STEP_INTERVAL_SECONDS
    value: "60"
  - name: MAX_INFLIGHT
    value: "512"
  - name: ENABLE_DYNAMIC_CHANNEL_POOL
    value: "true"
```

Continuous DCP scale-up/scale-down example:

```yaml
env:
  - name: PROBE_TYPE
    value: "stale_query"
  - name: START_QPS
    value: "200"
  - name: END_QPS
    value: "5000"
  - name: STEP_QPS_PERCENT
    value: "50"
  - name: QPS_STEP_INTERVAL_SECONDS
    value: "180"
  - name: QPS_CYCLE_ENABLED
    value: "true"
  - name: LOW_QPS_HOLD_SECONDS
    value: "600"
  - name: MAX_INFLIGHT
    value: "600"
  - name: ENABLE_DYNAMIC_CHANNEL_POOL
    value: "true"
```

See Kubernetes examples:

- `k8s/dcp_workload1_step_up_stale_query.yaml` — ramp-up scale-up workload
- `k8s/dcp_workload2_burst_stale_query.yaml` — burst scale-up workload
- `k8s/dcp_workload4_cycle_step_down_stale_query.yaml` — ramp-cycle scale-up/scale-down workload
