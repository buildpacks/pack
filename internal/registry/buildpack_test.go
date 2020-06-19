package registry_test

import (
	"testing"

	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	"github.com/buildpacks/pack/internal/registry"
	h "github.com/buildpacks/pack/testhelpers"
)

func TestRegistryBuildpack(t *testing.T) {
	spec.Run(t, "Buildpack", testRegistryBuildpack, spec.Parallel(), spec.Report(report.Terminal{}))
}

func testRegistryBuildpack(t *testing.T, when spec.G, it spec.S) {
	when("#Validate", func() {
		it("errors when address is missing", func() {
			b := registry.Buildpack{
				Address: "",
			}

			h.AssertNotNil(t, b.Validate())
		})

		it("errors when not a digest", func() {
			b := registry.Buildpack{
				Address: "example.com/some/package:18",
			}

			h.AssertNotNil(t, b.Validate())
		})

		it("does not error when address is a digest", func() {
			b := registry.Buildpack{
				Address: "example.com/some/package@sha256:8c27fe111c11b722081701dfed3bd55e039b9ce92865473cf4cdfa918071c566",
			}

			h.AssertNil(t, b.Validate())
		})
	})
}
