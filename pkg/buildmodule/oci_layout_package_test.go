package buildmodule_test

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
	"github.com/buildpacks/pack/pkg/buildmodule"
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
			mainBP, depBPs, err := buildmodule.BuildpacksFromOCILayoutBlob(blob.NewBlob(filepath.Join("testdata", "hello-universe.cnb")))
			h.AssertNil(t, err)

			h.AssertEq(t, mainBP.Descriptor().Info().ID, "io.buildpacks.samples.hello-universe")
			h.AssertEq(t, mainBP.Descriptor().Info().Version, "0.0.1")
			h.AssertEq(t, len(depBPs), 2)
		})

		it("provides readable blobs", func() {
			mainBP, depBPs, err := buildmodule.BuildpacksFromOCILayoutBlob(blob.NewBlob(filepath.Join("testdata", "hello-universe.cnb")))
			h.AssertNil(t, err)

			for _, bp := range append([]buildmodule.BuildModule{mainBP}, depBPs...) {
				reader, err := bp.Open()
				h.AssertNil(t, err)

				_, contents, err := archive.ReadTarEntry(
					reader,
					fmt.Sprintf("/cnb/buildpacks/%s/%s/buildpack.toml",
						bp.Descriptor().Info().ID,
						bp.Descriptor().Info().Version,
					),
				)
				h.AssertNil(t, err)
				h.AssertContains(t, string(contents), bp.Descriptor().Info().ID)
				h.AssertContains(t, string(contents), bp.Descriptor().Info().Version)
			}
		})
	})

	when.Pend("#ExtensionsFromOCILayoutBlob", func() { // TODO: add fixture when `pack extension package` is supported in https://github.com/buildpacks/pack/issues/1489
		it("extracts buildpacks", func() {
			ext, err := buildmodule.ExtensionsFromOCILayoutBlob(blob.NewBlob(filepath.Join("testdata", "hello-extensions.cnb")))
			h.AssertNil(t, err)

			h.AssertEq(t, ext.Descriptor().Info().ID, "io.buildpacks.samples.hello-extensions")
			h.AssertEq(t, ext.Descriptor().Info().Version, "0.0.1")
		})

		it("provides readable blobs", func() {
			ext, err := buildmodule.ExtensionsFromOCILayoutBlob(blob.NewBlob(filepath.Join("testdata", "hello-extensions.cnb")))
			h.AssertNil(t, err)

			reader, err := ext.Open()
			h.AssertNil(t, err)

			_, contents, err := archive.ReadTarEntry(
				reader,
				fmt.Sprintf("/cnb/extensions/%s/%s/extension.toml",
					ext.Descriptor().Info().ID,
					ext.Descriptor().Info().Version,
				),
			)
			h.AssertNil(t, err)
			h.AssertContains(t, string(contents), ext.Descriptor().Info().ID)
			h.AssertContains(t, string(contents), ext.Descriptor().Info().Version)
		})
	})

	when("#IsOCILayoutBlob", func() {
		when("is an OCI layout blob", func() {
			it("returns true", func() {
				isOCILayoutBlob, err := buildmodule.IsOCILayoutBlob(blob.NewBlob(filepath.Join("testdata", "hello-universe.cnb")))
				h.AssertNil(t, err)
				h.AssertEq(t, isOCILayoutBlob, true)
			})
		})

		when("is NOT an OCI layout blob", func() {
			it("returns false", func() {
				buildpackBlob, err := fakes.NewFakeBuildpackBlob(&dist.BuildpackDescriptor{
					WithAPI: api.MustParse("0.3"),
					WithInfo: dist.ModuleInfo{
						ID:      "bp.id",
						Version: "bp.version",
					},
					WithStacks: []dist.Stack{{}},
					WithOrder:  nil,
				}, 0755)
				h.AssertNil(t, err)

				isOCILayoutBlob, err := buildmodule.IsOCILayoutBlob(buildpackBlob)
				h.AssertNil(t, err)
				h.AssertEq(t, isOCILayoutBlob, false)
			})
		})
	})
}
