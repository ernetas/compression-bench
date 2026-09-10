// Command probe compresses one layer with one method and reports how many cores
// it actually used and what it cost in memory.
//
// compbench itself cannot answer either question for every method: throughput
// alone cannot distinguish "fast codec" from "codec using eight cores", and the
// mem/op column is Go-heap churn, which is blank for cgo and external methods
// whose memory lives off the Go heap or in a child process.
//
// One method per process, so ru_maxrss is that method's own high-water mark.
// RUSAGE_SELF covers pure-Go and cgo; RUSAGE_CHILDREN covers external CLIs.
//
//	go run ./hack/large-layer/cmd/probe -method kp-zstd -level default <layer.tar>
package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"sort"
	"syscall"
	"time"

	"github.com/tonistiigi/compression-bench/method"
)

func main() {
	name := flag.String("method", "kp-zstd", "registered method name")
	levelName := flag.String("level", "default", "fast|default|best")
	bufSize := flag.Int("buf", 1<<20, "copy buffer size")
	op := flag.String("op", "compress", "compress|decompress")
	out := flag.String("out", "", "compress: also write the artifact here, for a later -op decompress")
	flag.Parse()
	if flag.NArg() != 1 {
		fmt.Fprintln(os.Stderr, "usage: probe [-method m] [-level l] <layer.tar>")
		os.Exit(2)
	}

	m, ok := method.Get(*name)
	if !ok {
		// Not a built-in: fall back to the external CLIs the large-layer config
		// uses, so one flag name covers every row in that report.
		if m, ok = externalMethod(*name); !ok {
			fmt.Fprintf(os.Stderr, "unknown method %q (have %v plus %v)\n",
				*name, method.Names(), externalNames())
			os.Exit(1)
		}
	}
	level, err := method.ParseLevel(*levelName)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	src, err := os.Open(flag.Arg(0))
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	defer src.Close()
	fi, err := src.Stat()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	// One measured operation per process, so the rusage figures below belong to
	// it alone. Decompress reads an artifact a previous -op compress produced.
	counter := &countWriter{}
	var dst io.Writer = counter
	if *op == "compress" && *out != "" {
		f, err := os.Create(*out)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		defer f.Close()
		dst = io.MultiWriter(counter, f)
	}

	start := time.Now()
	switch *op {
	case "compress":
		err = method.Compress(m, dst, src, level, *bufSize)
	case "decompress":
		err = method.Decompress(m, dst, src, *bufSize)
	default:
		err = fmt.Errorf("unknown op %q", *op)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	wall := time.Since(start)

	cpu, rss := usage()
	// Throughput is always over the uncompressed side, so compress and
	// decompress rows for the same method are directly comparable.
	var uncompressed int64
	ratio := 0.0
	switch *op {
	case "compress":
		uncompressed = fi.Size()
		ratio = float64(fi.Size()) / float64(counter.count())
	case "decompress":
		uncompressed = counter.count()
		ratio = float64(counter.count()) / float64(fi.Size())
	}
	mib := float64(uncompressed) / (1 << 20)
	fmt.Printf("%-14s %-11s %-8s %8.2fs wall %9.2fs cpu %6.2fx cores %9.1f MiB rss %8.0f MB/s %7.3f ratio\n",
		*name, *op, *levelName, wall.Seconds(), cpu, cpu/wall.Seconds(), rss,
		mib/wall.Seconds(), ratio)
}

// usage sums this process and any children it waited for, so in-process (Go,
// cgo) and external methods are reported on the same basis. ru_maxrss is KiB on
// Linux and bytes on Darwin.
func usage() (cpuSeconds, peakRSSMiB float64) {
	var self, children syscall.Rusage
	syscall.Getrusage(syscall.RUSAGE_SELF, &self)
	syscall.Getrusage(syscall.RUSAGE_CHILDREN, &children)
	secs := func(r syscall.Rusage) float64 {
		return float64(r.Utime.Sec) + float64(r.Utime.Usec)/1e6 +
			float64(r.Stime.Sec) + float64(r.Stime.Usec)/1e6
	}
	div := 1024.0 // Linux reports KiB
	if maxrssIsBytes {
		div = 1024 * 1024
	}
	rss := float64(self.Maxrss)
	if float64(children.Maxrss) > rss {
		rss = float64(children.Maxrss)
	}
	return secs(self) + secs(children), rss / div
}

type countWriter struct{ n int64 }

func (c *countWriter) Write(p []byte) (int, error) { c.n += int64(len(p)); return len(p), nil }
func (c *countWriter) count() int64                { return c.n }

var _ io.Writer = (*countWriter)(nil)

// zstdCLILevels mirrors the rawLevels map in the large-layer config.
var zstdCLILevels = map[method.Level]string{method.Fast: "1", method.Default: "3", method.Best: "19"}

// externals mirrors the external entries of hack/large-layer/compbench-large.yaml.
// Each row pins its thread mode: the CLI is MT by default since 1.5.x, and -T1 is
// still the MT path with one worker -- only --single-thread avoids it, at a
// different output digest.
var externals = map[string]struct {
	cmd, decmd []string
	levels     map[method.Level]string
}{
	"pigz":        {[]string{"pigz", "-{level}", "-c"}, []string{"pigz", "-d", "-c"}, nil},
	"pigz-p1":     {[]string{"pigz", "-{level}", "-p", "1", "-c"}, []string{"pigz", "-d", "-c"}, nil},
	"zstd-cli-st": {[]string{"zstd", "-{level}", "--single-thread", "-c"}, []string{"zstd", "-d", "-c"}, zstdCLILevels},
	"zstd-cli":    {[]string{"zstd", "-{level}", "-T1", "-c"}, []string{"zstd", "-d", "-c"}, zstdCLILevels},
	"zstd-cli-mt": {[]string{"zstd", "-{level}", "-T0", "-c"}, []string{"zstd", "-d", "-c"}, zstdCLILevels},
}

func externalMethod(name string) (method.Method, bool) {
	e, ok := externals[name]
	if !ok {
		return nil, false
	}
	m, err := method.NewExternal(name, e.cmd, e.decmd, e.levels)
	if err != nil {
		return nil, false
	}
	return m, true
}

func externalNames() []string {
	out := make([]string, 0, len(externals))
	for n := range externals {
		out = append(out, n)
	}
	sort.Strings(out)
	return out
}
