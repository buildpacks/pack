package logging_test

import (
	"bytes"
	"fmt"
	"testing"

	"github.com/heroku/color"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	ilogging "github.com/buildpacks/pack/internal/logging"
	"github.com/buildpacks/pack/logging"
	h "github.com/buildpacks/pack/testhelpers"
)

func TestLogging(t *testing.T) {
	color.Disable(true)
	defer color.Disable(false)
	spec.Run(t, "logging", testLogging, spec.Parallel(), spec.Report(report.Terminal{}))
}

func testLogging(t *testing.T, when spec.G, it spec.S) {
	when("#GetWriterForLevel", func() {
		when("implements WithSelectableWriter", func() {
			it("returns Logger for appropriate level", func() {
				outCons, output := h.MockWriterAndOutput()
				errCons, errOutput := h.MockWriterAndOutput()
				logger := ilogging.NewLogWithWriters(outCons, errCons)

				infoLogger := logging.GetWriterForLevel(logger, logging.InfoLevel)
				h.AssertSameInstance(t, infoLogger, outCons)
				_, _ = infoLogger.Write([]byte("info test"))
				h.AssertEq(t, output(), "info test")

				errorLogger := logging.GetWriterForLevel(logger, logging.ErrorLevel)
				h.AssertSameInstance(t, errorLogger, errCons)
				_, _ = errorLogger.Write([]byte("error test"))
				h.AssertEq(t, errOutput(), "error test")
			})
		})

		when("doesn't implement WithSelectableWriter", func() {
			it("returns one Writer for all levels", func() {
				var w bytes.Buffer
				logger := logging.New(&w)
				writer := logging.GetWriterForLevel(logger, logging.InfoLevel)
				_, _ = writer.Write([]byte("info test\n"))
				h.AssertEq(t, w.String(), "info test\n")

				writer = logging.GetWriterForLevel(logger, logging.ErrorLevel)
				_, _ = writer.Write([]byte("error test\n"))
				h.AssertEq(t, w.String(), "info test\nerror test\n")
			})
		})
	})

	when("PrefixWriter#Write", func() {
		it("prepends prefix to string", func() {
			var w bytes.Buffer
			prefix := "test prefix"
			writer := logging.NewPrefixWriter(&w, prefix)
			_, _ = writer.Write([]byte("test"))
			h.AssertEq(t, w.String(), fmt.Sprintf("[%s] %s", prefix, "test"))
		})
	})

	when("#Tip", func() {
		it("prepends `Tip:` to string", func() {
			var w bytes.Buffer
			logger := logging.New(&w)
			logging.Tip(logger, "test")
			h.AssertContains(t, w.String(), "Tip: "+"test")
		})
	})
}
