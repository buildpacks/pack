package buildpack_test

import (
	"testing"

	"github.com/buildpacks/lifecycle/api"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	ifakes "github.com/buildpacks/pack/internal/fakes"
	"github.com/buildpacks/pack/pkg/buildpack"
	"github.com/buildpacks/pack/pkg/dist"
	h "github.com/buildpacks/pack/testhelpers"
)

func TestFlattener(t *testing.T) {
	spec.Run(t, "Flattener", testFlattener, spec.Report(report.Terminal{}))
}

func testFlattener(t *testing.T, when spec.G, it spec.S) {
	var (
		bp1 buildpack.BuildModule
		bp2 buildpack.BuildModule
		bp3 buildpack.BuildModule
		err error
	)

	it.Before(func() {
		bp1, err = ifakes.NewFakeBuildpack(dist.BuildpackDescriptor{
			WithAPI: api.MustParse("0.2"),
			WithInfo: dist.ModuleInfo{
				ID:      "buildpack-1-id",
				Version: "buildpack-1-version",
			},
		}, 0644)
		h.AssertNil(t, err)

		bp2, err = ifakes.NewFakeBuildpack(dist.BuildpackDescriptor{
			WithAPI: api.MustParse("0.2"),
			WithInfo: dist.ModuleInfo{
				ID:      "buildpack-2-id",
				Version: "buildpack-2-version",
			},
		}, 0644)
		h.AssertNil(t, err)

		bp3, err = ifakes.NewFakeBuildpack(dist.BuildpackDescriptor{
			WithAPI: api.MustParse("0.2"),
			WithInfo: dist.ModuleInfo{
				ID:      "buildpack-3-id",
				Version: "buildpack-3-version",
			},
		}, 0644)
		h.AssertNil(t, err)
	})

	when("Flattener has been called", func() {
		var (
			flattener buildpack.BuildpacksFlattener
		)
		it.Before(func() {
			flattener = buildpack.NewBuildpacksFlattener()
		})

		it("flats the buildpacks that has been passed", func() {
			bps := flattener.FlatBuildpacks([]buildpack.BuildModule{bp1, bp2, bp3})
			h.AssertEq(t, len(bps), 1)
		})
	})
}
