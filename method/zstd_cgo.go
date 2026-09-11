//go:build cgo_zstd

package method

import (
	"io"
	"runtime"

	ddzstd "github.com/DataDog/zstd"
	"github.com/pkg/errors"
)

// This file is compiled only with `-tags cgo_zstd` (and CGO_ENABLED=1). It is
// additive: the cgo binding is registered *alongside* the pure-Go kp-zstd so a
// single run compares both on the same corpus. See zstd_cgo_stub.go for the
// pure-Go build, which registers nothing.

func init() {
	Register(zstdCgo{})
	if parallelCgoZstdSupported() {
		Register(zstdCgoMT{})
	}
}

// parallelCgoZstdSupported reports whether the linked libzstd was built with
// multithreading. SetNbWorkers returns ErrNoParallelSupport against a shared
// library compiled without it, in which case the MT variant is left unregistered
// and the run records it as a skip rather than failing.
func parallelCgoZstdSupported() bool {
	w := ddzstd.NewWriterLevel(io.Discard, 1)
	defer w.Close()
	return w.SetNbWorkers(2) == nil
}

func cgoZstdLevel(l Level) int {
	switch l {
	case Fast:
		return 1
	case Best:
		return 19
	default:
		return 3
	}
}

type zstdCgo struct{}

func (zstdCgo) Name() string            { return "zstd-cgo" }
func (zstdCgo) Version() string         { return modVersion("github.com/DataDog/zstd") }
func (zstdCgo) RawLevel(l Level) string { return zstdRawLevel(l) }
func (zstdCgo) GoMemory() bool          { return false } // libzstd memory is off the Go heap

func (zstdCgo) NewWriter(w io.Writer, level Level) (io.WriteCloser, error) {
	return ddzstd.NewWriterLevel(w, cgoZstdLevel(level)), nil
}

func (zstdCgo) NewReader(r io.Reader) (io.ReadCloser, error) {
	return ddzstd.NewReader(r), nil
}

// zstdCgoMT is the same binding with ZSTD_c_nbWorkers set, i.e. libzstd's own
// multithreading -- the in-process equivalent of `zstd -T0`. Without it the
// zstd-cgo row is a one-worker baseline by omission rather than by constraint:
// the binding does expose the parameter, via SetNbWorkers.
type zstdCgoMT struct{}

func (zstdCgoMT) Name() string            { return "zstd-cgo-mt" }
func (zstdCgoMT) Version() string         { return modVersion("github.com/DataDog/zstd") }
func (zstdCgoMT) RawLevel(l Level) string { return zstdRawLevel(l) }
func (zstdCgoMT) GoMemory() bool          { return false }

func (zstdCgoMT) NewWriter(w io.Writer, level Level) (io.WriteCloser, error) {
	enc := ddzstd.NewWriterLevel(w, cgoZstdLevel(level))
	// GOMAXPROCS, matching what pgzip and klauspost's zstd default to, so the
	// rows are comparable. Note this cannot be 0: at the library level
	// ZSTD_c_nbWorkers=0 means *no* worker threads, not "detect cores" -- the
	// core detection behind the CLI's -T0 lives in the CLI, not in libzstd.
	// Passing 0 here silently yields a single-threaded encoder.
	if err := enc.SetNbWorkers(runtime.GOMAXPROCS(0)); err != nil {
		enc.Close()
		return nil, errors.Wrap(err, "zstd-cgo-mt: SetNbWorkers")
	}
	return enc, nil
}

func (zstdCgoMT) NewReader(r io.Reader) (io.ReadCloser, error) {
	return ddzstd.NewReader(r), nil
}
