package buildpack

import (
	"strings"
	"testing"

	"github.com/heroku/color"
	"github.com/opencontainers/go-digest"
	v1 "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"
)

func TestOCILayoutPackageInternal(t *testing.T) {
	color.Disable(true)
	defer color.Disable(false)
	spec.Run(t, "GetLayer", testOCILayoutPackageGetLayer, spec.Parallel(), spec.Report(report.Terminal{}))
}

// This is an internal (whitebox) test because GetLayer is an unexported method on
// the unexported ociLayoutPackage type; the external buildpack_test package cannot
// construct one directly.
func testOCILayoutPackageGetLayer(t *testing.T, when spec.G, it spec.S) {
	when("#GetLayer", func() {
		when("the config rootfs references a layer index beyond the manifest layers", func() {
			it("returns an error instead of panicking", func() {
				// The config rootfs lists two diffIDs but the manifest only carries a
				// single layer, so resolving the second diffID yields index 1 which is
				// out of range for manifest.Layers. Before the bounds check this
				// panicked with index out of range; it should now return an error.
				layer0 := digest.FromString("layer-0")
				outOfRangeDiffID := digest.FromString("layer-with-no-manifest-entry")

				pkg := &ociLayoutPackage{
					imageInfo: v1.Image{
						RootFS: v1.RootFS{
							DiffIDs: []digest.Digest{layer0, outOfRangeDiffID},
						},
					},
					manifest: v1.Manifest{
						Layers: []v1.Descriptor{
							{Digest: layer0},
						},
					},
				}

				_, err := pkg.GetLayer(outOfRangeDiffID.String())
				if err == nil {
					t.Fatal("expected an error but got nil")
				}
				if want := "the manifest only has 1 layer(s)"; !strings.Contains(err.Error(), want) {
					t.Fatalf("expected error to contain %q, got %q", want, err.Error())
				}
			})
		})
	})
}
