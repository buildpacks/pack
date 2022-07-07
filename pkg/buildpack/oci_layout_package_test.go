package buildpack_test

import (
	"fmt"
	"path/filepath"
	"testing"

	"github.com/buildpacks/lifecycle/api"
	"github.com/heroku/color"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	"github.com/buildpacks/pack/internal/fakes"
	"github.com/buildpacks/pack/pkg/archive"
	"github.com/buildpacks/pack/pkg/blob"
	"github.com/buildpacks/pack/pkg/buildpack"
	"github.com/buildpacks/pack/pkg/dist"
	h "github.com/buildpacks/pack/testhelpers"
)

func TestOCILayoutPackage(t *testing.T) {
	color.Disable(true)
	defer color.Disable(false)
	spec.Run(t, "Extract", testOCILayoutPackage, spec.Parallel(), spec.Report(report.Terminal{}))
}

func testOCILayoutPackage(t *testing.T, when spec.G, it spec.S) {
	when("#BuildpacksFromOCILayoutBlob", func() {
		it("extracts buildpacks", func() {
			mainBP, depBPs, err := buildpack.BuildpacksFromOCILayoutBlob(blob.NewBlob(filepath.Join("testdata", "hello-universe.cnb")))
			h.AssertNil(t, err)

			h.AssertEq(t, mainBP.Descriptor().ModuleInfo().ID, "io.buildpacks.samples.hello-universe")
			h.AssertEq(t, mainBP.Descriptor().ModuleInfo().Version, "0.0.1")
			h.AssertEq(t, len(depBPs), 2)
		})

		it("provides readable blobs", func() {
			mainBP, depBPs, err := buildpack.BuildpacksFromOCILayoutBlob(blob.NewBlob(filepath.Join("testdata", "hello-universe.cnb")))
			h.AssertNil(t, err)

			for _, bp := range append([]buildpack.Buildpack{mainBP}, depBPs...) {
				reader, err := bp.Open()
				h.AssertNil(t, err)

				_, contents, err := archive.ReadTarEntry(
					reader,
					fmt.Sprintf("/cnb/buildpacks/%s/%s/buildpack.toml",
						bp.Descriptor().ModuleInfo().ID,
						bp.Descriptor().ModuleInfo().Version,
					),
				)
				h.AssertNil(t, err)
				h.AssertContains(t, string(contents), bp.Descriptor().ModuleInfo().ID)
				h.AssertContains(t, string(contents), bp.Descriptor().ModuleInfo().Version)
			}
		})
	})

	when("#IsOCILayoutBlob", func() {
		when("is an OCI layout blob", func() {
			it("returns true", func() {
				isOCILayoutBlob, err := buildpack.IsOCILayoutBlob(blob.NewBlob(filepath.Join("testdata", "hello-universe.cnb")))
				h.AssertNil(t, err)
				h.AssertEq(t, isOCILayoutBlob, true)
			})
		})

		when("is NOT an OCI layout blob", func() {
			it("returns false", func() {
				buildpackBlob, err := fakes.NewFakeBuildpackBlob(&dist.BuildpackDescriptor{
					API: api.MustParse("0.3"),
					Info: dist.BuildpackInfo{
						ID:      "bp.id",
						Version: "bp.version",
					},
					Stacks: []dist.Stack{{}},
					Order:  nil,
				}, 0755)
				h.AssertNil(t, err)

				isOCILayoutBlob, err := buildpack.IsOCILayoutBlob(buildpackBlob)
				h.AssertNil(t, err)
				h.AssertEq(t, isOCILayoutBlob, false)
			})
		})
	})
}
