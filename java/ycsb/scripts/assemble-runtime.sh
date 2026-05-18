#!/usr/bin/env bash
set -euo pipefail

if [[ $# -ne 2 ]]; then
  echo "Usage: $0 <ycsb-root> <output-dir>" >&2
  exit 1
fi

YCSB_ROOT="$1"
OUT="$2"

rm -rf "${OUT}"
mkdir -p \
  "${OUT}/lib" \
  "${OUT}/bin" \
  "${OUT}/cloudspanner-binding/lib" \
  "${OUT}/cloudspanner-binding/conf" \
  "${OUT}/cloudspanner/conf" \
  "${OUT}/workloads"

CORE_JAR="$(find "${YCSB_ROOT}/core/target" -maxdepth 1 -type f -name 'core-*.jar' ! -name '*-tests.jar' ! -name 'original-*' | head -n 1)"
test -n "${CORE_JAR}"
cp "${CORE_JAR}" "${OUT}/lib/core.jar"

BINDING_JAR="$(find "${YCSB_ROOT}/cloudspanner/target" -maxdepth 1 -type f -name 'cloudspanner-binding-*.jar' ! -name '*-tests.jar' ! -name 'original-*' | head -n 1)"
test -n "${BINDING_JAR}"
cp "${BINDING_JAR}" "${OUT}/cloudspanner-binding/lib/cloudspanner-binding.jar"

mvn -B -ntp -f "${YCSB_ROOT}/core/pom.xml" dependency:copy-dependencies \
  -DincludeScope=runtime \
  -DoutputDirectory="${OUT}/lib" \
  -DskipTests

cp "${YCSB_ROOT}/bin/ycsb" "${OUT}/bin/ycsb"
chmod +x "${OUT}/bin/ycsb"
cp -r "${YCSB_ROOT}/cloudspanner/conf/." "${OUT}/cloudspanner-binding/conf/"
cp -r "${YCSB_ROOT}/cloudspanner/conf/." "${OUT}/cloudspanner/conf/"
cp -r "${YCSB_ROOT}/workloads/." "${OUT}/workloads/"
cp "${YCSB_ROOT}/LICENSE.txt" "${OUT}/LICENSE.txt"
cp "${YCSB_ROOT}/NOTICE.txt" "${OUT}/NOTICE.txt"
