package logging_test

import (
	"bytes"
	"fmt"
	"testing"

	"github.com/heroku/color"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	"github.com/buildpacks/pack/logging"
	h "github.com/buildpacks/pack/testhelpers"
)

func TestPrefixWriter(t *testing.T) {
	color.Disable(true)
	defer color.Disable(false)
	spec.Run(t, "PrefixWriter", testPrefixWriter, spec.Parallel(), spec.Report(report.Terminal{}))
}

func testPrefixWriter(t *testing.T, when spec.G, it spec.S) {
	when("#Write", func() {
		it("prepends prefix to string", func() {
			var w bytes.Buffer
			prefix := "test prefix"
			writer := logging.NewPrefixWriter(&w, prefix)
			_, _ = writer.Write([]byte("test"))
			_ = writer.Close()

			h.AssertEq(t, w.String(), fmt.Sprintf("[%s] %s", prefix, "test"))
		})

		it("prepends prefix to multi-line string", func() {
			var w bytes.Buffer

			writer := logging.NewPrefixWriter(&w, "prefix")
			_, _ = writer.Write([]byte("line 1\nline 2\nline 3"))
			_ = writer.Close()
			h.AssertEq(t, w.String(), "[prefix] line 1\n[prefix] line 2\n[prefix] line 3")
		})

		it("buffers mid-line calls", func() {
			var buf bytes.Buffer

			writer := logging.NewPrefixWriter(&buf, "prefix")
			_, _ = writer.Write([]byte("word 1, "))
			_, _ = writer.Write([]byte("word 2, "))
			_, _ = writer.Write([]byte("word 3."))
			_ = writer.Close()

			h.AssertEq(t, buf.String(), "[prefix] word 1, word 2, word 3.")
		})
	})
}
