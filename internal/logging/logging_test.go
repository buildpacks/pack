package logging

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
	lw.Logger.Level = log.InfoLevel
	return &lw
}

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
		var logger *logWithWriters
		var outCons, errCons *color.Console
		var fOut func() string

		it.Before(func() {
			outCons, fOut = mockStd()
			errCons, _ = mockStd()
			logger = newTestLogger(outCons, errCons)
		})

		it.After(func() {
		})

		it("can enable time in logs", func() {
			logger.WantTime(true)
			logger.Error("test")
			expected := "2019/05/15 01:01:01.000000 \x1b[31;1mERROR: \x1b[0mtest\n"
			h.AssertEq(t, fOut(), expected)
		})

		it("it has no time and color by default", func() {
			logger.Error("test")
			expected := "\x1b[31;1mERROR: \x1b[0mtest\n"
			h.AssertEq(t, fOut(), expected)
		})

		it("can disable color logs", func() {
			outCons.DisableColors(true)
			logger.Error("test")
			expected := "ERROR: test\n"
			h.AssertEq(t, fOut(), expected)
		})

		it("non-error levels not shown", func() {
			logger.Info("test")
			expected := "test\n"
			h.AssertEq(t, fOut(), expected)
		})

		it("will not show verbose messages if quiet", func() {
			logger.WantQuiet(true)
			logger.Debug("hello")
			logger.Debugf("there")
			logger.Info("test")
			logger.Warn("oh no")
			expected := "oh no\n"
			h.AssertContains(t, fOut(), expected)

			testOut := logger.Writer()
			h.AssertSameInstance(t, testOut, outCons)

			testOut = logger.InfoErrorWriter()
			h.AssertSameInstance(t, testOut, ioutil.Discard)
		})

		it("will return correct writers", func() {
			testOut := logger.Writer()
			h.AssertSameInstance(t, testOut, outCons)
			testOut = logger.InfoErrorWriter()
			h.AssertSameInstance(t, testOut, errCons)
		})

		it("will convert an empty string to a line feed", func() {
			logger.Info("")
			expected := "\n"
			h.AssertEq(t, fOut(), expected)
		})
	})
}
