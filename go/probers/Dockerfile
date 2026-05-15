# Build from spanner-client-libraries-prober repo root.
# Release example:
#   docker build -f go/probers/Dockerfile --target runtime-release \
#     --build-arg SPANNER_VERSION=v1.91.0 -t irahul-go-client:v1.91.0 .
# Source branch example:
#   docker build -f go/probers/Dockerfile --target runtime-source \
#     --build-arg GCGO_REPO_URL=https://github.com/rahul2393/google-cloud-go.git \
#     --build-arg GCGO_REF=my-fix-branch -t irahul-go-client:my-fix-branch .

FROM golang:1.25 AS release-build

ARG SPANNER_VERSION=v1.91.0
WORKDIR /workspace/prober/go/probers

COPY go/probers/go.mod go/probers/go.sum ./
RUN go mod download
COPY go/probers ./
RUN go get "cloud.google.com/go/spanner@${SPANNER_VERSION}" \
 && go mod tidy \
 && CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags="-s -w" -o /out/spanner-prober .

FROM golang:1.25 AS source-build

ARG GCGO_REPO_URL=https://github.com/rahul2393/google-cloud-go.git
ARG GCGO_REF=main
WORKDIR /workspace

RUN apt-get update \
 && apt-get install -y --no-install-recommends ca-certificates git \
 && rm -rf /var/lib/apt/lists/*

COPY go/probers /workspace/prober/go/probers

RUN git clone --depth 1 --branch "${GCGO_REF}" "${GCGO_REPO_URL}" /workspace/google-cloud-go \
 || (git clone "${GCGO_REPO_URL}" /workspace/google-cloud-go \
     && cd /workspace/google-cloud-go \
     && git checkout "${GCGO_REF}")

WORKDIR /workspace/prober/go/probers
RUN go mod edit -replace cloud.google.com/go/spanner=/workspace/google-cloud-go/spanner \
 && go mod tidy \
 && CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags="-s -w" -o /out/spanner-prober .

FROM gcr.io/distroless/base-debian12 AS runtime-release

WORKDIR /app
COPY --from=release-build /out/spanner-prober /app/spanner-prober
ENTRYPOINT ["/app/spanner-prober"]

FROM gcr.io/distroless/base-debian12 AS runtime-source

WORKDIR /app
COPY --from=source-build /out/spanner-prober /app/spanner-prober
ENTRYPOINT ["/app/spanner-prober"]
