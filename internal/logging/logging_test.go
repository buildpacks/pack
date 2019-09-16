package logging

import (
	"bytes"
	"io"
	"io/ioutil"
	"runtime"
	"testing"
	"time"

	"github.com/apex/log"
	"github.com/fatih/color"
	"github.com/sclevine/spec"

	h "github.com/buildpack/pack/testhelpers"
)

const testTime = "2019/05/15 01:01:01.000000"

func newTestLogger(stdout, stderr io.Writer) *logWithWriters {
	hnd := &handler{
		writer: stdout,
		timer: func() time.Time {
			tm, _ := time.Parse(timeFmt, testTime)
			return tm
		},
	}
	var lw logWithWriters
	lw.handler = hnd
	lw.out = hnd.writer
	lw.errOut = stderr
	lw.Logger.Handler = hnd
	lw.Logger.Level = log.DebugLevel
	return &lw
}

func TestPackCLILogger(t *testing.T) {
	spec.Run(t, "PackCLILogger", func(t *testing.T, when spec.G, it spec.S) {
		var log, errLog bytes.Buffer
		var logger *logWithWriters

		it.Before(func() {
			color.NoColor = false
			logger = newTestLogger(&log, &errLog)
		})

		it.After(func() {
			log.Reset()
			errLog.Reset()
		})

		it("can enable time in logs", func() {
			logger.WantTime(true)
			logger.Error("test")
			expected := "2019/05/15 01:01:01.000000 \x1b[31;1mERROR: \x1b[0mtest\n"
			h.AssertEq(t, log.String(), expected)
		})

		it("it has no time and color by default", func() {
			logger.Error("test")
			expected := "\x1b[31;1mERROR: \x1b[0mtest\n"
			h.AssertEq(t, log.String(), expected)
		})

		it("can disable color logs", func() {
			color.NoColor = true
			logger.Error("test")
			expected := "ERROR: test\n"
			h.AssertEq(t, log.String(), expected)
		})

		it("non-error levels not shown", func() {
			logger.Info("test")
			expected := "test\n"
			h.AssertEq(t, log.String(), expected)
		})

		it("will not show verbose messages if quiet", func() {
			logger.WantQuiet(true)
			logger.Debug("hello")
			logger.Debugf("there")
			h.AssertEq(t, log.String(), "")
			logger.Info("test")
			expected := "test\n"
			h.AssertEq(t, log.String(), expected)

			testOut := logger.Writer()
			h.AssertSameInstance(t, testOut, &log)

			testOut = logger.DebugErrorWriter()
			h.AssertSameInstance(t, testOut, ioutil.Discard)
		})

		it("will return correct writers", func() {
			testOut := logger.Writer()
			h.AssertSameInstance(t, testOut, &log)

			testOut = logger.DebugErrorWriter()
			h.AssertSameInstance(t, testOut, &errLog)
		})

		it("will convert an empty string to a line feed", func() {
			logger.Info("")
			expected := "\n"
			h.AssertEq(t, log.String(), expected)
		})
	})
}

func TestColor(t *testing.T) {
	spec.Run(t, "writer", func(t *testing.T, when spec.G, it spec.S) {
		var previousColorSetting bool
		var wtr *Writer
		var out bytes.Buffer

		it.Before(func() {
			previousColorSetting = color.NoColor
			wtr = New(&out)
		})

		it.After(func() {
			color.NoColor = previousColorSetting
			out.Reset()
		})

		when("color enabled", func() {
			it.Before(func() {
				if runtime.GOOS == "windows" {
					t.Skip("with color tests disabled on windows")
				}
				color.NoColor = false
			})

			it("should add color", func() {
				writeLines(wtr,
					color.RedString("line one "),
					color.YellowString("line two\n"),
				)
				want := "\x1b[31mline one \x1b[0m\x1b[33mline two\n\x1b[0m\n"
				got := out.String()
				h.AssertEq(t, got, want)
			})

			it("should handle split lines", func() {
				writeLines(wtr,
					color.RedString("one\ntwo"),
					"\nthree",
				)
				want := "\x1b[31mone\ntwo\x1b[0m\nthree\n"
				got := out.String()
				h.AssertEq(t, got, want)
			})

			it("should handle empty input", func() {
				writeLines(wtr, "")
				want := ""
				got := out.String()
				h.AssertEq(t, got, want)
			})

			it("should handle single line", func() {
				writeLines(wtr, "a line\n")
				want := "a line\n"
				got := out.String()
				h.AssertEq(t, got, want)
			})

		})

		when("color disabled", func() {
			it.Before(func() {
				color.NoColor = true
			})

			it("should strip color", func() {
				writeLines(wtr, color.RedString("line one "), color.YellowString("line two\n"))
				want := "line one line two\n"
				got := out.String()
				h.AssertEq(t, got, want)
			})

			it("handles split lines", func() {
				writeLines(wtr, color.RedString("one\ntwo"), "\nthree")
				want := "one\ntwo\nthree\n"
				got := out.String()
				h.AssertEq(t, got, want)
			})
		})
	})
}

func writeLines(w io.WriteCloser, lines ...string) {
	for _, line := range lines {
		_, _ = w.Write([]byte(line))
	}
	_ = w.Close()
}

func TestMaybeStripColors(t *testing.T) {
	spec.Run(t, "combinations", func(t *testing.T, when spec.G, it spec.S) {
		var originalColorSetting bool
		it.Before(func() {
			originalColorSetting = color.NoColor
			color.NoColor = true
		})
		it.After(func() {
			color.NoColor = originalColorSetting
		})
		it("should strip color", func() {
			baseAttrs := []color.Attribute{
				color.Reset,
				color.Bold,
				color.Faint,
				color.Italic,
				color.Underline,
				color.BlinkSlow,
				color.BlinkRapid,
				color.ReverseVideo,
				color.Concealed,
				color.CrossedOut,
			}
			foregroundAttrs := []color.Attribute{
				color.FgBlack,
				color.FgRed,
				color.FgGreen,
				color.FgYellow,
				color.FgBlue,
				color.FgMagenta,
				color.FgCyan,
				color.FgWhite,
				color.FgHiBlack,
				color.FgHiRed,
				color.FgHiGreen,
				color.FgHiYellow,
				color.FgHiBlue,
				color.FgHiMagenta,
				color.FgHiCyan,
				color.FgHiWhite,
			}
			backgroundAttrs := []color.Attribute{
				color.BgBlack,
				color.BgRed,
				color.BgGreen,
				color.BgYellow,
				color.BgBlue,
				color.BgMagenta,
				color.BgCyan,
				color.BgWhite,
				color.BgHiBlack,
				color.BgHiRed,
				color.BgHiGreen,
				color.BgHiYellow,
				color.BgHiBlue,
				color.BgHiMagenta,
				color.BgHiCyan,
				color.BgHiWhite,
			}
			want := "Hello, 786 colorful\n[ ]; world."
			testCombo := func(c *color.Color) {
				var out bytes.Buffer
				_, _ = c.Fprint(&out, want)
				got := maybeStripColor(out.Bytes())
				if got != want {
					t.Logf("color %+v", *c)
					t.Fatalf("got %q want %q", got, want)
				}
			}

			for i := 0; i < len(baseAttrs); i++ {
				testCombo(color.New(baseAttrs[i]))

				for j := 0; j < len(foregroundAttrs); j++ {
					testCombo(color.New(foregroundAttrs[j]))
					testCombo(color.New(baseAttrs[i], foregroundAttrs[j]))

					for k := 0; k < len(backgroundAttrs); k++ {
						testCombo(color.New(backgroundAttrs[k]))
						testCombo(color.New(backgroundAttrs[k], foregroundAttrs[j]))
						testCombo(color.New(backgroundAttrs[k], baseAttrs[i]))
						testCombo(color.New(baseAttrs[i], foregroundAttrs[j], backgroundAttrs[k]))
					}
				}
			}
		})

	})
}
