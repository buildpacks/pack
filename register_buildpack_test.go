package pack

import (
	"testing"

	"github.com/heroku/color"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"
)

func TestRegisterBuildpack(t *testing.T) {
	color.Disable(true)
	defer color.Disable(false)
	spec.Run(t, "rebase_factory", testRegisterBuildpack, spec.Parallel(), spec.Report(report.Terminal{}))
}

func testRegisterBuildpack(t *testing.T, when spec.G, it spec.S) {
}
