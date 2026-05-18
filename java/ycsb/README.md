# Java Spanner YCSB prober

Minimal YCSB tree for Cloud Spanner benchmarking. Non-Spanner datastore bindings are intentionally removed.

## Build released google-cloud-spanner image

```bash
./java/ycsb/scripts/build-images.sh release 6.117.0
```

This uses Maven Central artifact `com.google.cloud:google-cloud-spanner:<version>`.

## Build image from unreleased google-cloud-java branch/SHA

Push branch first, then:

```bash
./java/ycsb/scripts/build-images.sh source <branch-or-sha>
```

Defaults:

```bash
GCJ_REPO_URL=https://github.com/rahul2393/google-cloud-java.git
IMAGE_REPO=us-central1-docker.pkg.dev/span-cloud-testing/irahul-images/irahul-ycsb
```

Override example:

```bash
GCJ_REPO_URL=https://github.com/googleapis/google-cloud-java.git \
  ./java/ycsb/scripts/build-images.sh source main
```

Push image:

```bash
./java/ycsb/scripts/build-images.sh release 6.117.0 --push
./java/ycsb/scripts/build-images.sh source directpath-fixes-all --push
```

## Direct Docker commands

Release:

```bash
docker build \
  -f java/ycsb/Dockerfile \
  --target runtime-release \
  --build-arg SPANNER_VERSION=6.117.0 \
  -t us-central1-docker.pkg.dev/span-cloud-testing/irahul-images/irahul-ycsb:6.117.0 \
  .
```

Source branch/SHA:

```bash
docker build \
  -f java/ycsb/Dockerfile \
  --target runtime-source \
  --build-arg GCJ_REPO_URL=https://github.com/rahul2393/google-cloud-java.git \
  --build-arg GCJ_REF=<branch-or-sha> \
  -t us-central1-docker.pkg.dev/span-cloud-testing/irahul-images/irahul-ycsb:<branch-tag> \
  .
```

## Run

```bash
docker run --rm <image> run cloudspanner \
  -P workloads/workloadb \
  -P cloudspanner/conf/cloudspanner.properties \
  -p recordcount=1000000000 \
  -p operationcount=3000000 \
  -threads 300 \
  -target 45000 \
  -s
```

Kubernetes examples live in `java/ycsb/k8s/`.
