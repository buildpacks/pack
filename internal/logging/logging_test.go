package logging_test

import (
	"bytes"
	"io"
	"io/ioutil"
	"os"
	"testing"
	"time"

	"github.com/heroku/color"
	"github.com/sclevine/spec"

	ilogging "github.com/buildpacks/pack/internal/logging"
	h "github.com/buildpacks/pack/testhelpers"
)

const (
	timeFmt  = "2006/01/02 15:04:05.000000"
	testTime = "2019/05/15 01:01:01.000000"
)

func mockStd() (*color.Console, func() string) {
	r, w, _ := os.Pipe()
	console := color.NewConsole(w)
	return console, func() string {
		_ = w.Close()
		var b bytes.Buffer
		_, _ = io.Copy(&b, r)
		_ = r.Close()
		return b.String()
	}
}

func TestPackCLILogger(t *testing.T) {
	spec.Run(t, "PackCLILogger", func(t *testing.T, when spec.G, it spec.S) {
		var (
			logger           *ilogging.LogWithWriters
			outCons, errCons *color.Console
			fOut, fErr       func() string
		)

		it.Before(func() {
			outCons, fOut = mockStd()
			errCons, fErr = mockStd()
			logger = ilogging.NewLogWithWriters(outCons, errCons, ilogging.WithClock(func() time.Time {
				clock, _ := time.Parse(timeFmt, testTime)
				return clock
			}))
		})

		when("default", func() {
			it("has no time and color", func() {
				logger.Info(color.HiBlueString("test"))
				h.AssertEq(t, fOut(), "\x1b[94mtest\x1b[0m\n")
			})

			it("logs error to error writer", func() {
				logger.Error("oh no")

				h.AssertContains(t, fErr(), "oh no\n")
			})

			it("will return correct writers", func() {
				h.AssertSameInstance(t, logger.Writer(), outCons)
				h.AssertSameInstance(t, logger.OutWriter(), outCons)
				h.AssertSameInstance(t, logger.ErrorWriter(), errCons)
			})
		})

		when("time is set to true", func() {
			it("time is logged", func() {
				logger.WantTime(true)
				logger.Info("test")
				h.AssertEq(t, fOut(), "2019/05/15 01:01:01.000000 test\n")
			})
		})

		when("colors are disabled", func() {
			it("don't display colors", func() {
				outCons.DisableColors(true)
				logger.Info(color.HiBlueString("test"))
				h.AssertEq(t, fOut(), "test\n")
			})
		})

		when("quiet is set to true", func() {
			it.Before(func() {
				logger.WantQuiet(true)
			})

			it("will not show debug or info messages", func() {
				logger.Debug("hello")
				logger.Debugf("there")
				logger.Info("test")

				h.AssertContains(t, fOut(), "")
			})

			it("logs warnings to standard writer", func() {
				logger.Warn("oh no")

				h.AssertContains(t, fOut(), "oh no\n")
			})

			it("logs error to error writer", func() {
				logger.Error("oh no")

				h.AssertContains(t, fErr(), "oh no\n")
			})

			it("will return correct writers", func() {
				h.AssertSameInstance(t, logger.Writer(), outCons)
				h.AssertSameInstance(t, logger.OutWriter(), ioutil.Discard)
				h.AssertSameInstance(t, logger.ErrorWriter(), errCons)
			})
		})

		it("will convert an empty string to a line feed", func() {
			logger.Info("")
			expected := "\n"
			h.AssertEq(t, fOut(), expected)
		})
	})
}
