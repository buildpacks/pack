package builder

import (
	"testing"

	"github.com/fatih/color"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	h "github.com/buildpack/pack/testhelpers"
)

func TestCompat(t *testing.T) {
	color.NoColor = true
	spec.Run(t, "Compat", testCompat, spec.Parallel(), spec.Report(report.Terminal{}))
}

func testCompat(t *testing.T, when spec.G, it spec.S) {
	when("#v1OrderTOMLFromOrderTOML", func() {
		it("converts", func() {
			newToml := orderTOML{
				Order: []GroupConfig{
					{
						Group: []BuildpackRefConfig{
							{ID: "buildpack.id.1", Version: "1.2.3", Optional: false},
							{ID: "buildpack.id.2", Version: "4.5.6", Optional: true},
						},
					},
				},
			}

			result := v1OrderTOMLFromOrderTOML(newToml)

			h.AssertEq(t, result, v1OrderTOML{
				Groups: []v1Group{
					{
						Buildpacks: []v1BuildpackRef{
							{ID: "buildpack.id.1", Version: "1.2.3", Optional: false},
							{ID: "buildpack.id.2", Version: "4.5.6", Optional: true},
						},
					},
				},
			})
		})
	})
}
