package method

import (
	"io"

	stdgzip "compress/gzip"

	kpgzip "github.com/klauspost/compress/gzip"
	"github.com/klauspost/pgzip"
	"github.com/pkg/errors"
)

// pgzipBlockSize is klauspost/pgzip's own default block size. The compressed
// bytes are a function of this alone — not of the worker count — so every pgzip
// variant that keeps it produces byte-identical output.
const pgzipBlockSize = 1 << 20

// gzipLevel maps the normalized level onto the gzip 1..9 scale, shared by every
// gzip-family method (stdlib, klauspost, pgzip).
func gzipRawLevel(l Level) string {
	switch l {
	case Fast:
		return "1"
	case Best:
		return "9"
	default:
		return "6"
	}
}

func gzipNativeLevel(l Level) int {
	switch l {
	case Fast:
		return 1
	case Best:
		return 9
	default:
		return 6
	}
}

func init() {
	Register(stdlibGzip{})
	Register(kpGzip{})
	Register(kpPgzip{})
	Register(kpPgzipB1{})
}

// stdlibGzip is compress/gzip, the single-threaded baseline.
type stdlibGzip struct{}

func (stdlibGzip) Name() string            { return "stdlib-gzip" }
func (stdlibGzip) Version() string         { return "stdlib" }
func (stdlibGzip) RawLevel(l Level) string { return gzipRawLevel(l) }
func (stdlibGzip) GoMemory() bool          { return true }

func (stdlibGzip) NewWriter(w io.Writer, level Level) (io.WriteCloser, error) {
	gw, err := stdgzip.NewWriterLevel(w, gzipNativeLevel(level))
	return gw, errors.WithStack(err)
}

func (stdlibGzip) NewReader(r io.Reader) (io.ReadCloser, error) {
	gr, err := stdgzip.NewReader(r)
	return gr, errors.WithStack(err)
}

// kpGzip is klauspost/compress/gzip, single-threaded.
type kpGzip struct{}

func (kpGzip) Name() string            { return "kp-gzip" }
func (kpGzip) Version() string         { return modVersion("github.com/klauspost/compress") }
func (kpGzip) RawLevel(l Level) string { return gzipRawLevel(l) }
func (kpGzip) GoMemory() bool          { return true }

func (kpGzip) NewWriter(w io.Writer, level Level) (io.WriteCloser, error) {
	gw, err := kpgzip.NewWriterLevel(w, gzipNativeLevel(level))
	return gw, errors.WithStack(err)
}

func (kpGzip) NewReader(r io.Reader) (io.ReadCloser, error) {
	gr, err := kpgzip.NewReader(r)
	return gr, errors.WithStack(err)
}

// kpPgzip is klauspost/pgzip, in-process goroutine-parallel gzip. Internal
// concurrency is left at the library default on purpose.
type kpPgzip struct{}

func (kpPgzip) Name() string            { return "kp-pgzip" }
func (kpPgzip) Version() string         { return modVersion("github.com/klauspost/pgzip") }
func (kpPgzip) RawLevel(l Level) string { return gzipRawLevel(l) }
func (kpPgzip) GoMemory() bool          { return true }

func (kpPgzip) NewWriter(w io.Writer, level Level) (io.WriteCloser, error) {
	gw, err := pgzip.NewWriterLevel(w, gzipNativeLevel(level))
	return gw, errors.WithStack(err)
}

func (kpPgzip) NewReader(r io.Reader) (io.ReadCloser, error) {
	gr, err := pgzip.NewReader(r)
	return gr, errors.WithStack(err)
}

// kpPgzipB1 is klauspost/pgzip with a single block in flight -- SetConcurrency's
// `blocks` set to 1, against a default of GOMAXPROCS. It is not sequential: pgzip
// always compresses on its own goroutine, so this measures ~2 cores, not 1.
//
// Its purpose is the memory knob, not the throughput. Output is byte-identical
// to kp-pgzip (the bytes are a function of block size alone), so the pair
// isolates what in-flight block count actually buys. Measured on a 13GB layer at
// level 6: 100.2 MiB peak RSS at 1639 MB/s (14.1x cores) against 33.8 MiB at
// 311 MB/s (2.0x cores) here.
type kpPgzipB1 struct{}

func (kpPgzipB1) Name() string            { return "kp-pgzip-b1" }
func (kpPgzipB1) Version() string         { return modVersion("github.com/klauspost/pgzip") }
func (kpPgzipB1) RawLevel(l Level) string { return gzipRawLevel(l) }
func (kpPgzipB1) GoMemory() bool          { return true }

func (kpPgzipB1) NewWriter(w io.Writer, level Level) (io.WriteCloser, error) {
	gw, err := pgzip.NewWriterLevel(w, gzipNativeLevel(level))
	if err != nil {
		return nil, errors.WithStack(err)
	}
	// Default block size, one block in flight. pgzip's default is
	// SetConcurrency(pgzipBlockSize, GOMAXPROCS).
	if err := gw.SetConcurrency(pgzipBlockSize, 1); err != nil {
		gw.Close()
		return nil, errors.WithStack(err)
	}
	return gw, nil
}

// NewReader deliberately uses the default reader rather than
// NewReaderN(pgzipBlockSize, 1). pgzip's reader `blocks` is readahead depth, not
// compression concurrency -- inflate is serial either way -- so it is not part
// of what this method varies, and holding the reader to a single pooled buffer
// fails the round-trip gate on multi-GB layers with "gzip: invalid checksum".
// Using the default also keeps the decompress rows comparable with kp-pgzip.
func (kpPgzipB1) NewReader(r io.Reader) (io.ReadCloser, error) {
	gr, err := pgzip.NewReader(r)
	return gr, errors.WithStack(err)
}
