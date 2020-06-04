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

func TestTimeWriter(t *testing.T) {
	spec.Run(t, "TimeWriter", testTimeWriter, spec.Parallel(), spec.Report(report.Terminal{}))
}

func testTimeWriter(t *testing.T, when spec.G, it spec.S) {
	var (
		writer  *ilogging.TimeWriter
		outCons *color.Console
		fOut    func() string
	)

	it.Before(func() {
		outCons, fOut = h.MockWriterAndOutput()
		clockFunc := func() time.Time {
			clock, _ := time.Parse(timeFmt, testTime)
			return clock
		}

		writer = ilogging.NewTimeWriter(outCons, clockFunc)
	})

	when("default", func() {
		it("has no time and color", func() {
			writer.Write([]byte("test\n"))
			h.AssertEq(t, fOut(), "2019/05/15 01:01:01.000000 test\n")
		})
	})
}
