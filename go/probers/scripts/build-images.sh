#!/usr/bin/env bash
set -euo pipefail

IMAGE_REPO="${IMAGE_REPO:-us-central1-docker.pkg.dev/span-cloud-testing/irahul-images/irahul-go-client}"
DOCKERFILE="${DOCKERFILE:-go/probers/Dockerfile}"
GCGO_REPO_URL="${GCGO_REPO_URL:-https://github.com/rahul2393/google-cloud-go.git}"
PUSH="${PUSH:-false}"
NO_CACHE="${NO_CACHE:-false}"

usage() {
  cat <<USAGE
Usage:
  $0 release <spanner-version> [--push] [--no-cache]
  $0 source <branch-or-sha> [--push] [--no-cache]

Env:
  IMAGE_REPO     default: ${IMAGE_REPO}
  DOCKERFILE     default: ${DOCKERFILE}
  GCGO_REPO_URL  default: ${GCGO_REPO_URL}
  PUSH           default: ${PUSH}
  NO_CACHE       default: ${NO_CACHE}

Examples:
  $0 release v1.91.0
  $0 source directpath-fixes-all
  $0 source support-dynamic-channel-pooling --no-cache --push
  GCGO_REPO_URL=https://github.com/googleapis/google-cloud-go.git $0 source main
USAGE
}

if [[ $# -eq 1 && ( "$1" == "-h" || "$1" == "--help" ) ]]; then
  usage
  exit 0
fi

if [[ $# -lt 2 ]]; then
  usage
  exit 1
fi

MODE="$1"
VALUE="$2"
shift 2

while [[ $# -gt 0 ]]; do
  case "$1" in
    --push) PUSH=true ;;
    --no-cache) NO_CACHE=true ;;
    -h|--help) usage; exit 0 ;;
    *) echo "Unknown arg: $1" >&2; usage; exit 1 ;;
  esac
  shift
done

if [[ ! -f "${DOCKERFILE}" || ! -d go/probers ]]; then
  echo "Run from spanner-client-libraries-prober repo root. Missing ${DOCKERFILE} or go/probers." >&2
  exit 1
fi

sanitize_tag() {
  echo "$1" | tr '/:@' '---' | tr -cd 'A-Za-z0-9_.-'
}

DOCKER_CACHE_ARGS=()
if [[ "${NO_CACHE}" == "true" ]]; then
  DOCKER_CACHE_ARGS+=(--no-cache)
fi

case "${MODE}" in
  release)
    TAG="${IMAGE_REPO}:${VALUE}"
    docker build \
      "${DOCKER_CACHE_ARGS[@]}" \
      -f "${DOCKERFILE}" \
      --target runtime-release \
      --build-arg "SPANNER_VERSION=${VALUE}" \
      -t "${TAG}" \
      .
    ;;
  source)
    SAFE_VALUE="$(sanitize_tag "${VALUE}")"
    TAG="${IMAGE_REPO}:${SAFE_VALUE}"
    docker build \
      "${DOCKER_CACHE_ARGS[@]}" \
      -f "${DOCKERFILE}" \
      --target runtime-source \
      --build-arg "GCGO_REPO_URL=${GCGO_REPO_URL}" \
      --build-arg "GCGO_REF=${VALUE}" \
      -t "${TAG}" \
      .
    ;;
  *)
    echo "Unknown mode: ${MODE}" >&2
    usage
    exit 1
    ;;
esac

if [[ "${PUSH}" == "true" ]]; then
  docker push "${TAG}"
fi

echo "Built ${TAG}"
