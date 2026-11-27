package container

import (
	"bytes"
	"io"
	"testing"

	"github.com/heroku/color"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	"github.com/buildpacks/pack/pkg/logging"
)

func TestContainerRun(t *testing.T) {
	color.Disable(true)
	defer color.Disable(false)

	spec.Run(t, "container/run", testContainerRun, spec.Report(report.Terminal{}), spec.Parallel())
}

// recordingWriteCloser models a writer owned by the caller that also happens to
// implement io.Closer, such as os.Stdout passed to pack via the logger.
type recordingWriteCloser struct {
	bytes.Buffer
	closed bool
}

func (w *recordingWriteCloser) Close() error {
	w.closed = true
	return nil
}

var _ io.WriteCloser = &recordingWriteCloser{}

func testContainerRun(t *testing.T, when spec.G, it spec.S) {
	when("#optionallyCloseWriter", func() {
		it("does not close writers owned by the caller", func() {
			writer := &recordingWriteCloser{}

			if err := optionallyCloseWriter(writer); err != nil {
				t.Fatalf("expected no error, got: %v", err)
			}

			// Closing a caller-owned writer (e.g. os.Stdout) is surprising and
			// breaks consumers using pack as a library. See #1214.
			if writer.closed {
				t.Fatal("expected caller-owned writer to remain open, but it was closed")
			}
		})

		it("flushes buffered output from a PrefixWriter", func() {
			var buf bytes.Buffer
			prefixWriter := logging.NewPrefixWriter(&buf, "some-prefix")

			// A line without a trailing newline stays buffered until the writer
			// is flushed.
			if _, err := prefixWriter.Write([]byte("buffered output")); err != nil {
				t.Fatalf("writing to prefix writer: %v", err)
			}
			if got := buf.String(); got != "" {
				t.Fatalf("expected partial line to stay buffered, got: %q", got)
			}

			if err := optionallyCloseWriter(prefixWriter); err != nil {
				t.Fatalf("expected no error, got: %v", err)
			}

			if got := buf.String(); !bytes.Contains([]byte(got), []byte("buffered output")) {
				t.Fatalf("expected buffered output to be flushed, got: %q", got)
			}
		})
	})
}
