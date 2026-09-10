// Command pgzipworkers reports pgzip's throughput, cores and peak RSS at a given
// number of blocks in flight, for comparison against zstd's nbWorkers curve.
//
// pgzip's in-flight memory is blocks * blockSize, against libzstd's
// nbWorkers * jobSize -- the two scale at very different rates, which is what
// moby/buildkit#6841 needs to know before letting either run at GOMAXPROCS
// across several concurrent layer exports.
//
//	PGZIP_BLOCKS=8 go run ./hack/large-layer/cmd/pgzipworkers <layer.tar> [gzip-level]
package main

import (
	"fmt"
	"io"
	"os"
	"strconv"
	"syscall"
	"time"

	"github.com/klauspost/pgzip"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: pgzipworkers <layer.tar> [gzip-level]")
		os.Exit(2)
	}
	blocks := 4
	if v := os.Getenv("PGZIP_BLOCKS"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		blocks = n
	}
	level := 6
	if len(os.Args) > 2 {
		n, err := strconv.Atoi(os.Args[2])
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		level = n
	}

	src, err := os.Open(os.Args[1])
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

	start := time.Now()
	w, err := pgzip.NewWriterLevel(io.Discard, level)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if err := w.SetConcurrency(1<<20, blocks); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if _, err := io.CopyBuffer(w, src, make([]byte, 1<<20)); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if err := w.Close(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	wall := time.Since(start)

	var ru syscall.Rusage
	syscall.Getrusage(syscall.RUSAGE_SELF, &ru)
	cpu := float64(ru.Utime.Sec) + float64(ru.Utime.Usec)/1e6 +
		float64(ru.Stime.Sec) + float64(ru.Stime.Usec)/1e6
	div := 1024.0
	if maxrssIsBytes {
		div = 1024 * 1024
	}
	mib := float64(fi.Size()) / (1 << 20)
	fmt.Printf("%7.2fs wall %8.2fs cpu %6.2fx cores %8.1f MiB rss %8.0f MB/s\n",
		wall.Seconds(), cpu, cpu/wall.Seconds(), float64(ru.Maxrss)/div, mib/wall.Seconds())
}
