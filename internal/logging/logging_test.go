package logging_test

import (
	"bytes"
	"io"
	"io/ioutil"
	"os"
	"testing"
	"time"

	"github.com/apex/log"
	"github.com/heroku/color"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	ilogging "github.com/buildpacks/pack/internal/logging"
	"github.com/buildpacks/pack/logging"
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

func TestLogWithWriters(t *testing.T) {
	spec.Run(t, "logWithWriters", testLogWithWriters, spec.Parallel(), spec.Report(report.Terminal{}))
}

func testLogWithWriters(t *testing.T, when spec.G, it spec.S) {
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

		it("will not log debug messages", func() {
			logger.Debug("debug_")
			logger.Debugf("debugf")

			output := fOut()
			h.AssertNotContains(t, output, "debug_\n")
			h.AssertNotContains(t, output, "debugf\n")
		})

		it("logs info and warning messages to standard writer", func() {
			logger.Info("info_")
			logger.Infof("infof")
			logger.Warn("warn_")
			logger.Warnf("warnf")

			output := fOut()
			h.AssertContains(t, output, "info_\n")
			h.AssertContains(t, output, "infof\n")
			h.AssertContains(t, output, "warn_\n")
			h.AssertContains(t, output, "warnf\n")
		})

		it("logs error to error writer", func() {
			logger.Error("error_")
			logger.Errorf("errorf")

			output := fErr()
			h.AssertContains(t, output, "error_\n")
			h.AssertContains(t, output, "errorf\n")
		})

		it("will return correct writers", func() {
			h.AssertSameInstance(t, logger.Writer(), outCons)
			h.AssertSameInstance(t, logger.WriterForLevel(logging.DebugLevel), ioutil.Discard)
			h.AssertSameInstance(t, logger.WriterForLevel(logging.InfoLevel), outCons)
			h.AssertSameInstance(t, logger.WriterForLevel(logging.WarnLevel), outCons)
			h.AssertSameInstance(t, logger.WriterForLevel(logging.ErrorLevel), errCons)
		})

		it("is only verbose for debug level", func() {
			h.AssertFalse(t, logger.IsVerbose())

			logger.Level = log.DebugLevel
			h.AssertTrue(t, logger.IsVerbose())
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

		it("will not log debug or info messages", func() {
			logger.Debug("debug_")
			logger.Debugf("debugf")
			logger.Info("info_")
			logger.Infof("infof")

			output := fOut()
			h.AssertNotContains(t, output, "debug_\n")
			h.AssertNotContains(t, output, "debugf\n")
			h.AssertNotContains(t, output, "info_\n")
			h.AssertNotContains(t, output, "infof\n")
		})

		it("logs warnings to standard writer", func() {
			logger.Warn("warn_")
			logger.Warnf("warnf")

			output := fOut()
			h.AssertContains(t, output, "warn_\n")
			h.AssertContains(t, output, "warnf\n")
		})

		it("logs error to error writer", func() {
			logger.Error("error_")
			logger.Errorf("errorf")

			output := fErr()
			h.AssertContains(t, output, "error_\n")
			h.AssertContains(t, output, "errorf\n")
		})

		it("will return correct writers", func() {
			h.AssertSameInstance(t, logger.Writer(), outCons)
			h.AssertSameInstance(t, logger.WriterForLevel(logging.DebugLevel), ioutil.Discard)
			h.AssertSameInstance(t, logger.WriterForLevel(logging.InfoLevel), ioutil.Discard)
			h.AssertSameInstance(t, logger.WriterForLevel(logging.WarnLevel), outCons)
			h.AssertSameInstance(t, logger.WriterForLevel(logging.ErrorLevel), errCons)
		})
	})

	when("verbose is set to true", func() {
		it.Before(func() {
			logger.WantVerbose(true)
		})

		it("all messages are logged", func() {
			logger.Debug("debug_")
			logger.Debugf("debugf")
			logger.Info("info_")
			logger.Infof("infof")
			logger.Warn("warn_")
			logger.Warnf("warnf")

			output := fOut()
			h.AssertContains(t, output, "debug_")
			h.AssertContains(t, output, "debugf")
			h.AssertContains(t, output, "info_")
			h.AssertContains(t, output, "infof")
			h.AssertContains(t, output, "warn_")
			h.AssertContains(t, output, "warnf")
		})

		it("logs error to error writer", func() {
			logger.Error("error_")
			logger.Errorf("errorf")

			output := fErr()
			h.AssertContains(t, output, "error_\n")
			h.AssertContains(t, output, "errorf\n")
		})

		it("will return correct writers", func() {
			h.AssertSameInstance(t, logger.Writer(), outCons)
			h.AssertSameInstance(t, logger.WriterForLevel(logging.DebugLevel), outCons)
			h.AssertSameInstance(t, logger.WriterForLevel(logging.InfoLevel), outCons)
			h.AssertSameInstance(t, logger.WriterForLevel(logging.WarnLevel), outCons)
			h.AssertSameInstance(t, logger.WriterForLevel(logging.ErrorLevel), errCons)
		})
	})

	it("will convert an empty string to a line feed", func() {
		logger.Info("")
		expected := "\n"
		h.AssertEq(t, fOut(), expected)
	})
}
