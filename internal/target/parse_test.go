package target_test

import (
	"testing"

	"github.com/heroku/color"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	"github.com/buildpacks/pack/internal/target"
	"github.com/buildpacks/pack/pkg/dist"
	h "github.com/buildpacks/pack/testhelpers"
)

func TestParseTargets(t *testing.T) {
	color.Disable(true)
	defer color.Disable(false)
	spec.Run(t, "ParseTargets", testParseTargets, spec.Parallel(), spec.Report(report.Terminal{}))
}

func testParseTargets(t *testing.T, when spec.G, it spec.S) {
	it.Before(func() {
		var err error
		h.AssertNil(t, err)
	})

	when("target#ParseTarget", func() {
		it("should throw an error when [os][/arch][/variant] is nil", func() {
			_,_, err := target.ParseTarget(":distro@version")
			h.AssertNotNil(t, err)
		})
		it("should parse target as expected", func() {
			output,_, err := target.ParseTarget("linux/arm/v6")
			h.AssertNil(t, err)
			h.AssertEq(t, output, dist.Target{
				OS:          "linux",
				Arch:        "arm",
				ArchVariant: "v6",
			})
		})
	})
	when("target#ParseTargets", func() {
		it("should throw an error when atleast one target throws error", func() {
			_,_, err := target.ParseTargets([]string{"linux/arm/v6", ":distro@version"})
			h.AssertNotNil(t, err)
		})
		it("should parse targets as expected", func() {
			output,_, err := target.ParseTargets([]string{"linux/arm/v6", "linux/amd:ubuntu@22.04;debian@8.10@10.06"})
			h.AssertNil(t, err)
			h.AssertEq(t, output, []dist.Target{
				{
					OS:          "linux",
					Arch:        "arm",
					ArchVariant: "v6",
				},
				{
					OS:   "linux",
					Arch: "amd",
					Distributions: []dist.Distribution{
						{
							Name:     "ubuntu",
							Versions: []string{"22.04"},
						},
						{
							Name:     "debian",
							Versions: []string{"8.10", "10.06"},
						},
					},
				},
			})
		})
	})
	when("target#ParseDistro", func() {
		it("should parse distro as expected", func() {
			output, _, err := target.ParseDistro("ubuntu@22.04@20.08")
			h.AssertEq(t, output, dist.Distribution{
				Name:     "ubuntu",
				Versions: []string{"22.04", "20.08"},
			})
			h.AssertNil(t, err)
		})
	})
	when("target#ParseDistros", func() {
		it("should parse distros as expected", func() {
			output, _, err := target.ParseDistros("ubuntu@22.04@20.08;debian@8.10@10.06")
			h.AssertEq(t, output, []dist.Distribution{
				{
					Name:     "ubuntu",
					Versions: []string{"22.04", "20.08"},
				},
				{
					Name:     "debian",
					Versions: []string{"8.10", "10.06"},
				},
			})
			h.AssertNil(t, err)
		})
		it("result should be nil", func() {
			output, _, err := target.ParseDistros("")
			h.AssertEq(t, output, []dist.Distribution(nil))
			h.AssertNil(t, err)
		})
	})
}
