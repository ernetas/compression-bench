#!/usr/bin/env bash
# Memory and throughput against worker count, for both parallel codecs.
#
# This is the curve moby/buildkit#6841 needs: its blocker is that a parallel
# compressor's in-flight memory scales with core count, so exporting several
# layers at once on a many-core builder could exhaust memory. Measuring the slope
# on one machine lets you extrapolate to a 128-core builder instead of guessing.
#
# libzstd's MT memory is roughly nbWorkers * jobSize; pgzip's is blocks * 1MiB,
# so they scale at very different rates and the crossover matters.
#
#   ./hack/large-layer/probe-workers.sh <layer.tar> [level]
set -euo pipefail

layer="${1:?usage: probe-workers.sh <layer.tar> [level]}"
level="${2:-default}"
raw_zstd=3; [ "${level}" = fast ] && raw_zstd=1; [ "${level}" = best ] && raw_zstd=19
raw_gzip=6; [ "${level}" = fast ] && raw_gzip=1; [ "${level}" = best ] && raw_gzip=9

echo "=== zstd -${raw_zstd} against nbWorkers (peak RSS is the whole process) ==="
for t in 1 2 4 8 16; do
  printf "  -T%-3s " "$t"
  python3 - "$t" "$raw_zstd" "$layer" <<'PY'
import resource, subprocess, sys, time
t, lvl, layer = sys.argv[1], sys.argv[2], sys.argv[3]
t0 = time.monotonic()
with open(layer, 'rb') as f:
    subprocess.run(["zstd", f"-{lvl}", f"-T{t}", "-c"], stdin=f, stdout=subprocess.DEVNULL, check=True)
w = time.monotonic() - t0
ru = resource.getrusage(resource.RUSAGE_CHILDREN)
cpu = ru.ru_utime + ru.ru_stime
print(f"{w:7.2f}s wall {cpu:8.2f}s cpu {cpu/w:6.2f}x cores {ru.ru_maxrss/1024:8.1f} MiB rss")
PY
done

echo "=== pgzip -${raw_gzip} against blocks in flight ==="
for b in 1 2 4 8 16; do
  printf "  blocks=%-3s " "$b"
  PGZIP_BLOCKS="$b" go run ./hack/large-layer/cmd/pgzipworkers "$layer" "$raw_gzip"
done
