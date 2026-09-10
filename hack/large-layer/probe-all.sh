#!/usr/bin/env bash
# Per-method cores-used and peak RSS on a single layer.
#
# compbench reports throughput and Go-heap churn. Neither tells you how many
# cores a method used, and the churn column is blank for cgo/external methods.
# This runs one process per method so ru_maxrss is that method's own high-water
# mark, and prints cpu/wall as the parallelism factor.
#
#   ./hack/large-layer/probe-all.sh /path/to/largest-layer.tar [level]
set -euo pipefail

layer="${1:?usage: probe-all.sh <layer.tar> [level]}"
level="${2:-default}"
here="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
repo="$(cd "${here}/../.." && pwd)"

echo "layer: ${layer} ($(du -h "${layer}" | cut -f1)), level=${level}, $(nproc) hardware threads"
echo

for m in stdlib-gzip kp-gzip kp-pgzip kp-pgzip-b1 kp-zstd zstd-cgo pigz zstd-cli zstd-cli-mt; do
  ( cd "${repo}" && go run -tags cgo_zstd ./hack/large-layer/cmd/probe \
      -method "${m}" -level "${level}" "${layer}" ) || echo "${m}: skipped"
done
