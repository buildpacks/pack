package buildpack_test

import (
	"testing"

	"github.com/heroku/color"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"
)

func TestModuleManager(t *testing.T) {
	color.Disable(true)
	defer color.Disable(false)
	spec.Run(t, "ModuleManager", testModuleManager, spec.Report(report.Terminal{}))
}

func testModuleManager(t *testing.T, when spec.G, it spec.S) {
	when("#TBD", func() {

	})
}
