package logging

import (
	"bytes"
	"io"
	"io/ioutil"
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
			expected := "2019/05/15 01:01:01.000000 \x1b[31;1mERROR: \x1b[0mtest                     \n"
			h.AssertEq(t, log.String(), expected)
		})

		it("it has no time and color by default", func() {
			logger.Error("test")
			expected := "\x1b[31;1mERROR: \x1b[0mtest                     \n"
			h.AssertEq(t, log.String(), expected)
		})

		it("can disable color logs", func() {
			color.NoColor = true
			logger.Error("test")
			expected := "ERROR: test                     \n"
			h.AssertEq(t, log.String(), expected)
		})

		it("non-error levels not shown", func() {
			logger.Info("test")
			expected := "test                     \n"
			h.AssertEq(t, log.String(), expected)
		})

		it("will not show verbose messages if quiet", func() {
			logger.WantQuiet(true)
			logger.Debug("hello")
			logger.Debugf("there")
			h.AssertEq(t, log.String(), "")
			logger.Info("test")
			expected := "test                     \n"
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
