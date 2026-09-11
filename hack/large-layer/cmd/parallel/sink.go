package main

import (
	"io"
	"time"
)

// throttledWriter paces writes to a byte rate, standing in for a slow output.
//
// The measured memory of a parallel codec assumes its output is drained as fast
// as it is produced. buildkit writes into the content store on disk, which is
// not that: when the sink stalls, libzstd's output buffer pool fills (it holds
// 2*nbWorkers+3 buffers of ~compressBound(jobSize)) and pgzip's result channel
// backs up, so in-flight memory converges on its structural ceiling rather than
// the steady-state figure a /dev/null sink reports. This makes that measurable.
type throttledWriter struct {
	w         io.Writer
	bytesPerS float64
	started   time.Time
	written   int64
}

func newThrottledWriter(w io.Writer, mbPerSec float64) *throttledWriter {
	return &throttledWriter{w: w, bytesPerS: mbPerSec * (1 << 20), started: time.Now()}
}

func (t *throttledWriter) Write(p []byte) (int, error) {
	n, err := t.w.Write(p)
	t.written += int64(n)
	if t.bytesPerS > 0 {
		// Sleep until the elapsed time matches what this many bytes "should"
		// have taken at the target rate.
		want := time.Duration(float64(t.written) / t.bytesPerS * float64(time.Second))
		if d := want - time.Since(t.started); d > 0 {
			time.Sleep(d)
		}
	}
	return n, err
}
