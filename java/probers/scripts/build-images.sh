#!/usr/bin/env bash
set -euo pipefail

IMAGE_REPO="${IMAGE_REPO:-us-central1-docker.pkg.dev/span-cloud-testing/irahul-images/irahul-client}"
DOCKERFILE="${DOCKERFILE:-java/probers/Dockerfile}"
GCJ_REPO_URL="${GCJ_REPO_URL:-https://github.com/rahul2393/google-cloud-java.git}"
PUSH="${PUSH:-false}"

usage() {
  cat <<USAGE
Usage:
  $0 release <spanner-version> [--push]
  $0 source <branch-or-sha> [--push]

Env:
  IMAGE_REPO    default: ${IMAGE_REPO}
  DOCKERFILE    default: ${DOCKERFILE}
  GCJ_REPO_URL  default: ${GCJ_REPO_URL}
  PUSH          default: ${PUSH}

Examples:
  $0 release 6.114.0
  $0 release 6.117.0 --push
  $0 source directpath-fixes-all
  GCJ_REPO_URL=https://github.com/googleapis/google-cloud-java.git $0 source main
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
    -h|--help) usage; exit 0 ;;
    *) echo "Unknown arg: $1" >&2; usage; exit 1 ;;
  esac
  shift
done

if [[ ! -f "${DOCKERFILE}" || ! -d java/probers ]]; then
  echo "Run from spanner-client-libraries-prober repo root. Missing ${DOCKERFILE} or java/probers." >&2
  exit 1
fi

sanitize_tag() {
  echo "$1" | tr '/:@' '---' | tr -cd 'A-Za-z0-9_.-'
}

case "${MODE}" in
  release)
    TAG="${IMAGE_REPO}:${VALUE}"
    docker build \
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
      -f "${DOCKERFILE}" \
      --target runtime-source \
      --build-arg "GCJ_REPO_URL=${GCJ_REPO_URL}" \
      --build-arg "GCJ_REF=${VALUE}" \
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
