package logging_test

import (
	"testing"
	"time"

	"github.com/heroku/color"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	ilogging "github.com/buildpacks/pack/internal/logging"
	h "github.com/buildpacks/pack/testhelpers"
)

const (
	timeFmt  = "2006/01/02 15:04:05.000000"
	testTime = "2019/05/15 01:01:01.000000"
)

func TestLogWriter(t *testing.T) {
	spec.Run(t, "LogWriter", testLogWriter, spec.Parallel(), spec.Report(report.Terminal{}))
}

func testLogWriter(t *testing.T, when spec.G, it spec.S) {
	var (
		writer  *ilogging.LogWriter
		outCons *color.Console
		fOut    func() string

		clockFunc = func() time.Time {
			clock, _ := time.Parse(timeFmt, testTime)
			return clock
		}
	)

	it.Before(func() {
		outCons, fOut = h.MockWriterAndOutput()
	})

	when("wantTime is true", func() {
		it("has time", func() {
			writer = ilogging.NewLogWriter(outCons, clockFunc, true)
			writer.Write([]byte("test\n"))
			h.AssertEq(t, fOut(), "2019/05/15 01:01:01.000000 test\n")
		})
	})

	when("wantTime is false", func() {
		it("doesn't have time", func() {
			writer = ilogging.NewLogWriter(outCons, clockFunc, false)
			writer.Write([]byte("test\n"))
			h.AssertEq(t, fOut(), "test\n")
		})
	})
}
