# Go Spanner probers

Go prober app for Spanner client library experiments. Same binary can run different operations through `PROBE_TYPE`.

Supported `PROBE_TYPE` values:

- `strong_read`
- `stale_read`
- `read_write`
- `rr_occ_dml`
- `dml`
- `blind_dml`
- `multi_blind_dml`
- `strong_query`
- `stale_query`
- `multi_use_ro_query`
- `write`
- `write_no_rp`


## pprof

pprof is available but disabled by default. Enable with:

```yaml
env:
  - name: PPROF_ENABLED
    value: "true"
  - name: PPROF_ADDR
    value: ":6060"
```

Then port-forward and inspect heap:

```bash
kubectl port-forward -n spanner-ns pod/<pod> 6060:6060
go tool pprof http://localhost:6060/debug/pprof/heap
```

## Load and DCP knobs

The Go runner already uses goroutine-per-probe execution. The dispatcher submits work at target QPS and uses `MAX_INFLIGHT` as a semaphore. When the client is saturated, new submissions are dropped and counted in stats. Use these knobs for any `PROBE_TYPE`.

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
| `MAX_INFLIGHT` or `PARALLELISM` | `8` | Max active probe operations |
| `LOG_INTERVAL_SECONDS` | `10` | Stats log interval |

| DCP Env | Default | Meaning |
| --- | --- | --- |
| `SPANNER_DCP_ENABLED` or `DCP_ENABLED` | `false` | Opt in to Spanner client DCP when the linked client library supports it |
| `SPANNER_DCP_INITIAL_CHANNELS` | client default | Initial DCP channels |
| `SPANNER_DCP_MIN_CHANNELS` | client default | Minimum DCP channels |
| `SPANNER_DCP_MAX_CHANNELS` | client default | Maximum DCP channels |
| `SPANNER_DCP_MAX_RPC_PER_CHANNEL` | client default | Scale-up threshold |
| `SPANNER_DCP_MIN_RPC_PER_CHANNEL` | client default | Scale-down low-load threshold |

If the prober is built against a Spanner client version without `DynamicChannelPoolConfig`, `SPANNER_DCP_ENABLED=true` is ignored and the prober logs `spanner_dcp_config_ignored=true`.

QPS controller modes:

- default ramp: `START_QPS` ramps up to `END_QPS` by `STEP_QPS_PERCENT`, then stays at `END_QPS`;
- burst: `START_QPS` jumps to `END_QPS` once after `BURST_AFTER_SECONDS`;
- ramp cycle: with `QPS_CYCLE_ENABLED=true` and `BURST_ENABLED=false`, QPS ramps up to `END_QPS`, steps down to `START_QPS`, holds `LOW_QPS_HOLD_SECONDS`, then repeats;
- burst cycle: with both `QPS_CYCLE_ENABLED=true` and `BURST_ENABLED=true`, QPS waits `BURST_AFTER_SECONDS`, jumps to `END_QPS`, holds `HIGH_QPS_HOLD_SECONDS`, resets to `START_QPS`, then repeats.

For DCP scale-down validation, keep `LOW_QPS_HOLD_SECONDS` long enough for the client scale-down check interval, consecutive low-load checks, and drain idle grace.

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
  - name: SPANNER_DCP_ENABLED
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
  - name: SPANNER_DCP_ENABLED
    value: "true"
```

See Kubernetes examples:

- `k8s/dcp_workload1_step_up_stale_query.yaml`
- `k8s/dcp_workload2_burst_stale_query.yaml`
- `k8s/dcp_workload4_cycle_step_down_stale_query.yaml`

## Build released cloud.google.com/go/spanner image

```bash
./go/probers/scripts/build-images.sh release v1.91.0
```

This uses Go module `cloud.google.com/go/spanner@<version>`.

## Build image from unreleased google-cloud-go branch/SHA

Push branch first, then:

```bash
./go/probers/scripts/build-images.sh source <branch-or-sha>
```

This clones `google-cloud-go`, then runs:

```bash
go mod edit -replace cloud.google.com/go/spanner=/workspace/google-cloud-go/spanner
```

Defaults:

```bash
GCGO_REPO_URL=https://github.com/rahul2393/google-cloud-go.git
IMAGE_REPO=us-central1-docker.pkg.dev/span-cloud-testing/irahul-images/irahul-go-client
```

Override example:

```bash
GCGO_REPO_URL=https://github.com/googleapis/google-cloud-go.git \
  ./go/probers/scripts/build-images.sh source main
```

Push image:

```bash
./go/probers/scripts/build-images.sh release v1.91.0 --push
./go/probers/scripts/build-images.sh source directpath-fixes-all --push
```
