#!/usr/bin/env bash
# Per-method cores-used and peak RSS, for compress AND decompress.
#
# compbench reports throughput and Go-heap churn. Neither tells you how many
# cores a method used, and the churn column is blank for cgo/external methods.
# This runs one process per measured operation so ru_maxrss is that operation's
# own high-water mark, and prints cpu/wall as the parallelism factor.
#
# Decompress matters more than the compress numbers suggest: a layer is
# compressed once and decompressed on every pull, and inflate cannot be
# parallelised at all -- so a parallel gzip buys nothing on that side.
#
#   ./hack/large-layer/probe-all.sh <layer.tar> [level] [artifact-dir]
set -euo pipefail

layer="${1:?usage: probe-all.sh <layer.tar> [level] [artifact-dir]}"
level="${2:-default}"
artdir="${3:-$(mktemp -d)}"
here="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
repo="$(cd "${here}/../.." && pwd)"
mkdir -p "${artdir}"

echo "layer: ${layer} ($(du -h "${layer}" | cut -f1)), level=${level}, $(nproc) hardware threads"
echo

cd "${repo}"
for m in stdlib-gzip kp-gzip kp-pgzip kp-pgzip-b1 kp-zstd zstd-cgo zstd-cgo-mt pigz zstd-cli-st zstd-cli zstd-cli-mt; do
  art="${artdir}/${m}.bin"
  go run -tags cgo_zstd ./hack/large-layer/cmd/probe \
      -method "${m}" -level "${level}" -op compress -out "${art}" "${layer}" || { echo "${m}: skipped"; continue; }
  go run -tags cgo_zstd ./hack/large-layer/cmd/probe \
      -method "${m}" -level "${level}" -op decompress "${art}" || echo "${m}: decompress skipped"
  rm -f "${art}"
done
