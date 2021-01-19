package pack_test

import (
	"github.com/heroku/color"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"
	"testing"
)

func TestCreateBuildpack(t *testing.T) {
	color.Disable(true)
	defer color.Disable(false)
	spec.Run(t, "create_builder", testCreateBuildpack, spec.Parallel(), spec.Report(report.Terminal{}))
}

func testCreateBuildpack(t *testing.T, when spec.G, it spec.S) {
	when("#CreateBuildpack", func() {

	})
})
