#!/usr/bin/env bash
# Builds the large node_modules image and serves it from a throwaway local
# registry, so `compbench prep` can pull it like any other image. crane treats a
# localhost: registry as plain HTTP, so no TLS setup is needed.
#
#   ./hack/large-layer/build.sh            # ~11GiB target
#   TARGET_GIB=20 ./hack/large-layer/build.sh
#
# Then:  ./compbench run --config hack/large-layer/compbench-large.yaml
set -euo pipefail

TARGET_GIB="${TARGET_GIB:-11}"
REGISTRY_PORT="${REGISTRY_PORT:-5000}"
REF="localhost:${REGISTRY_PORT}/node-modules-monorepo:${TARGET_GIB}g"
here="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

if ! curl -sf -o /dev/null "http://localhost:${REGISTRY_PORT}/v2/"; then
  echo "==> starting local registry on :${REGISTRY_PORT}"
  docker rm -f compbench-registry >/dev/null 2>&1 || true
  docker run -d --name compbench-registry -p "${REGISTRY_PORT}:5000" registry:2 >/dev/null
  for _ in $(seq 1 30); do
    curl -sf -o /dev/null "http://localhost:${REGISTRY_PORT}/v2/" && break
    sleep 1
  done
fi

echo "==> building ${REF} (this installs real npm trees; expect several minutes)"
docker build --build-arg "TARGET_GIB=${TARGET_GIB}" -t "${REF}" "${here}"

echo "==> pushing ${REF}"
docker push "${REF}"

docker image inspect "${REF}" --format '{{.Size}}' |
  awk '{printf "==> image is %.2f GiB uncompressed\n", $1/1073741824}'
echo "==> now run:  ./compbench run --config hack/large-layer/compbench-large.yaml"
