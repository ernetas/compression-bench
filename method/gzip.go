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
	Register(kpPgzipSeq{})
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

// kpPgzipSeq is klauspost/pgzip held to one block in flight: the same
// block-parallel gzip format, compressed serially. It is the pgzip analogue of
// `zstd -T1`, and it splits pgzip's two effects apart — output is byte-identical
// to kp-pgzip (so the block-boundary ratio cost shows up here too), while
// throughput and in-flight memory are those of a single worker.
type kpPgzipSeq struct{}

func (kpPgzipSeq) Name() string            { return "kp-pgzip-seq" }
func (kpPgzipSeq) Version() string         { return modVersion("github.com/klauspost/pgzip") }
func (kpPgzipSeq) RawLevel(l Level) string { return gzipRawLevel(l) }
func (kpPgzipSeq) GoMemory() bool          { return true }

func (kpPgzipSeq) NewWriter(w io.Writer, level Level) (io.WriteCloser, error) {
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

func (kpPgzipSeq) NewReader(r io.Reader) (io.ReadCloser, error) {
	gr, err := pgzip.NewReaderN(r, pgzipBlockSize, 1)
	return gr, errors.WithStack(err)
}
