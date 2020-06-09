package pack

import (
	"github.com/heroku/color"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"
	"testing"
)

func TestPublishBuildpack(t *testing.T) {
	color.Disable(true)
	defer color.Disable(false)
	spec.Run(t, "rebase_factory", testPublishBuildpack, spec.Parallel(), spec.Report(report.Terminal{}))
}

func testPublishBuildpack(t *testing.T, when spec.G, it spec.S) {
	return
}