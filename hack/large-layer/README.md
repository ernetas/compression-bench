# Large-layer corpus

The default corpus (`alpine`, `golang`) has small, fairly even layers. That hides
the axis this benchmark is really being used to answer — whether a codec
parallelises **inside** one layer or only **across** layers — because compbench
times the *makespan* of a whole image:

    aggregate_throughput <= per_layer_throughput * (total_bytes / largest_layer)

With one dominant layer that ceiling collapses to roughly `per_layer_throughput`,
no matter how high you push `jobConcurrency`. Real images are shaped that way: a
`node_modules` install, a CUDA/PyTorch layer, a model download.

## Usage

```sh
./hack/large-layer/build.sh                      # build + serve the image locally
./compbench run --config hack/large-layer/compbench-large.yaml
./compbench report --results results-large/
```

`build.sh` starts a throwaway `registry:2` on `localhost:5000` and pushes there;
crane speaks plain HTTP to a `localhost:` registry, so no TLS setup is needed.
Override the size with `TARGET_GIB=20 ./hack/large-layer/build.sh` (and update
the `images:` ref in the config to match the tag).

Requires ~40GB of free disk: the image in the docker store, the same content in
the registry, and the decompressed corpus.

## What the image is

A monorepo whose apps each carry their own `node_modules` — twelve distinct real
dependency sets (Next, Angular, Vue, Nest, webpack, jest, Playwright + browsers,
Cypress, Electron, AWS SDK, eslint/prettier, three/d3), installed in a single
`RUN` and then replicated to reach the target size. Millions of small text files
plus a few hundred MB of browser and Electron binaries, which is what makes it a
fair stand-in for a real fat application layer rather than synthetic noise.

The replication is the one artificial part, and it is deliberate: a monorepo with
N app workspaces each holding its own `node_modules` is exactly the shape that
produces multi-GB layers in practice. It does not distort ratio, because the
duplicate content sits far beyond any of these codecs' match windows (32KiB for
deflate, ~1-4MiB for zstd at level 3).

Built size: ~13.7GiB across 10 layers, the largest ~13GB (~92% of the bytes).
