// Command parallel measures AGGREGATE memory when several layers are compressed
// at once in one process -- the "core count x layer count" case, which is the
// stated blocker in moby/buildkit#6841.
//
// Per-writer numbers do not answer it: buildkit's computeBlobChain fans out with
// no limit, so what matters is what N concurrent writers cost together, and
// whether handing each writer a share of a fixed budget actually holds the total
// down. -blocks 0 uses pgzip's default (GOMAXPROCS per writer), which is the
// behaviour being questioned; -budget divides a fixed number of in-flight blocks
// across the writers instead.
//
//	go run ./hack/large-layer/cmd/parallel -writers 8 -blocks 0   <layer.tar>
//	go run ./hack/large-layer/cmd/parallel -writers 8 -budget 16  <layer.tar>
package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"runtime"
	"sync"
	"syscall"
	"time"

	"github.com/klauspost/pgzip"
)

func main() {
	writers := flag.Int("writers", 4, "concurrent layer compressions")
	blocks := flag.Int("blocks", 0, "blocks in flight per writer; 0 = pgzip default (GOMAXPROCS)")
	budget := flag.Int("budget", 0, "total blocks in flight across all writers; overrides -blocks")
	level := flag.Int("level", 6, "gzip level")
	flag.Parse()
	if flag.NArg() != 1 {
		fmt.Fprintln(os.Stderr, "usage: parallel [-writers N] [-blocks B | -budget T] <layer.tar>")
		os.Exit(2)
	}

	perWriter := *blocks
	if perWriter == 0 {
		perWriter = runtime.GOMAXPROCS(0)
	}
	label := fmt.Sprintf("blocks=%d/writer", perWriter)
	if *budget > 0 {
		perWriter = max(1, *budget / *writers)
		label = fmt.Sprintf("budget=%d -> %d/writer", *budget, perWriter)
	}

	fi, err := os.Stat(flag.Arg(0))
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	start := time.Now()
	var wg sync.WaitGroup
	errs := make([]error, *writers)
	for i := range *writers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			errs[i] = compressOnce(flag.Arg(0), *level, perWriter)
		}()
	}
	wg.Wait()
	wall := time.Since(start)
	for _, e := range errs {
		if e != nil {
			fmt.Fprintln(os.Stderr, e)
			os.Exit(1)
		}
	}

	var ru syscall.Rusage
	syscall.Getrusage(syscall.RUSAGE_SELF, &ru)
	cpu := float64(ru.Utime.Sec) + float64(ru.Utime.Usec)/1e6 +
		float64(ru.Stime.Sec) + float64(ru.Stime.Usec)/1e6
	div := 1024.0
	if maxrssIsBytes {
		div = 1024 * 1024
	}
	total := float64(fi.Size()) * float64(*writers) / (1 << 20)
	fmt.Printf("%2d writers %-24s %7.2fs wall %6.2fx cores %9.1f MiB rss %8.0f MB/s aggregate\n",
		*writers, label, wall.Seconds(), cpu/wall.Seconds(), float64(ru.Maxrss)/div, total/wall.Seconds())
}

func compressOnce(path string, level, blocks int) error {
	src, err := os.Open(path)
	if err != nil {
		return err
	}
	defer src.Close()
	w, err := pgzip.NewWriterLevel(io.Discard, level)
	if err != nil {
		return err
	}
	if err := w.SetConcurrency(1<<20, blocks); err != nil {
		return err
	}
	if _, err := io.CopyBuffer(w, src, make([]byte, 1<<20)); err != nil {
		w.Close()
		return err
	}
	return w.Close()
}
