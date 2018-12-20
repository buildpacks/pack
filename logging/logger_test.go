package logging_test

import (
	"bytes"
	"fmt"
	"github.com/buildpack/pack/logging"
	"github.com/buildpack/pack/style"
	"strings"
	"testing"

	"github.com/fatih/color"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	h "github.com/buildpack/pack/testhelpers"
)

func TestLogger(t *testing.T) {
	color.NoColor = false // IMPORTANT: Keep this to avoid false positive tests
	spec.Run(t, "logging", testLogging, spec.Parallel(), spec.Report(report.Terminal{}))
}

func testLogging(t *testing.T, when spec.G, it spec.S) {

	var (
		logger *logging.Logger
		outBuf bytes.Buffer
		errBuf bytes.Buffer
	)

	when("verbosity", func() {
		when("logger has verbose enabled", func() {
			it.Before(func() {
				logger = logging.NewLogger(&outBuf, &errBuf, true, false)
			})

			it("shows verbose output", func() {
				logger.Verbose("Some verbose output")

				h.AssertEq(t, ignoreEmptyTimestampColorCodes(outBuf.String()), "Some verbose output\n")
			})

			it("returns real out writer", func() {
				writer := logger.VerboseWriter()
				writer.Write([]byte("Some text\n"))
				h.AssertEq(t, ignoreEmptyTimestampColorCodes(outBuf.String()), "Some text\n")
			})

			it("returns real err writer", func() {
				writer := logger.VerboseErrorWriter()
				writer.Write([]byte("Some error\n"))
				h.AssertEq(t, ignoreEmptyTimestampColorCodes(errBuf.String()), "Some error\n")
			})
		})

		when("logger has verbose disabled", func() {
			it.Before(func() {
				logger = logging.NewLogger(&outBuf, &errBuf, false, false)
			})

			it("does not show verbose output", func() {
				logger.Verbose("Some verbose output")

				h.AssertEq(t, outBuf.String(), "")
			})

			it("returns discard out writer", func() {
				writer := logger.VerboseWriter()
				writer.Write([]byte("some-text"))
				h.AssertEq(t, outBuf.String(), "")
			})

			it("returns discard err writer", func() {
				writer := logger.VerboseErrorWriter()
				writer.Write([]byte("some-text\n"))
				h.AssertEq(t, errBuf.String(), "")
			})
		})
	})

	when("timestamps", func() {
		when("logger has timestamps enabled", func() {
			it.Before(func() {
				logger = logging.NewLogger(&outBuf, &errBuf, false, true)
			})

			it("prefixes logging with timestamp", func() {
				logger.Info("Some text")
				h.AssertMatch(t, outBuf.String(), fmt.Sprintf(`^\x1b\[%dm\d{4}/\d{2}/\d{2} \d{2}:\d{2}:\d{2} \x1b\[%dm Some text`, style.TimestampColorCode, color.Reset))
			})
		})

		when("logger has timestamps disabled", func() {
			it.Before(func() {
				logger = logging.NewLogger(&outBuf, &errBuf, false, false)
			})

			it("does not prefix logging with timestamp", func() {
				logger.Info("Some text")

				h.AssertEq(t, ignoreEmptyTimestampColorCodes(outBuf.String()), "Some text\n")
			})
		})
	})

	when("styling", func() {
		it.Before(func() {
			logger = logging.NewLogger(&outBuf, &errBuf, true, false)
		})

		when("#Info", func() {
			it("displays unstyled info message", func() {
				logger.Info("This is some info")

				h.AssertEq(t, ignoreEmptyTimestampColorCodes(outBuf.String()), "This is some info\n")
			})
		})

		when("#Verbose", func() {
			it("displays unstyled verbose message", func() {
				logger.Verbose("This is some verbose text")

				h.AssertEq(t, ignoreEmptyTimestampColorCodes(outBuf.String()), "This is some verbose text\n")
			})
		})

		when("#Error", func() {
			it("displays styled error message to error buffer", func() {
				logger.Error("Something went wrong!")

				h.AssertEq(t, ignoreEmptyTimestampColorCodes(errBuf.String()), style.Error("ERROR: ")+"Something went wrong!\n")
			})
		})

		when("#Tip", func() {
			it("displays styled tip message", func() {
				logger.Tip("This is a tip")

				h.AssertEq(t, ignoreEmptyTimestampColorCodes(outBuf.String()), style.Tip("Tip: ")+"This is a tip\n")
			})
		})
	})

	when("#WithPrefix", func() {
		it("returns prefixed writer", func() {
			writer := logging.NewLogger(&outBuf, &errBuf, true, false).VerboseWriter()
			writer.WithPrefix("Some prefix").Write([]byte("Some text\n"))
			h.AssertEq(t, ignoreEmptyTimestampColorCodes(outBuf.String()), fmt.Sprintf("[%s] Some text\n", style.Prefix("Some prefix")))
		})
	})
}

func ignoreEmptyTimestampColorCodes(s string) string {
	// These codes are inserted, but have no timestamp between them
	return strings.TrimPrefix(s, fmt.Sprintf("\x1b[%dm\x1b[0m", style.TimestampColorCode))
}
