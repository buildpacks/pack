package buildpackage_test

import (
	"testing"

	"github.com/buildpack/imgutil/fakes"
	"github.com/heroku/color"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	"github.com/buildpack/pack/internal/buildpackage"
	"github.com/buildpack/pack/internal/dist"
	h "github.com/buildpack/pack/testhelpers"
)

func TestPackageImage(t *testing.T) {
	color.Disable(true)
	defer func() { color.Disable(false) }()
	spec.Run(t, "testPackageImage", testPackageImage, spec.Parallel(), spec.Report(report.Terminal{}))
}

func testPackageImage(t *testing.T, when spec.G, it spec.S) {
	var (
		image *fakes.Image
	)

	it.Before(func() {
		image = fakes.NewImage("some/image", "", nil)
	})
	
	when("#NewImage", func() {
		it("returns an instance when image is valid", func() {
			
			packageImage, err := buildpackage.NewImage(image)

			h.AssertNil(t, err)
			h.AssertNotNil(t, packageImage)
			
			packageImage.Metadata()
		})

		it("returns an error when metadata is not parsable from image", func() {
			h.AssertNil(t, image.SetLabel("io.buildpacks.buildpackage.metadata", "not json"))

			_, err := buildpackage.NewImage(image)
			
			h.AssertError(t, err, "unmarshalling label 'io.buildpacks.buildpackage.metadata' from image 'some/image'")
		})
	})

	when("#Metadata", func() {
		it("returns metadata from image label", func() {
			h.AssertNil(t, image.SetLabel("io.buildpacks.buildpackage.metadata", `{"id": "some.buildpack.id", "version": "some.buildpack.version", "stacks": [{"id": "some.stack.id", "mixins": ["some-mixin"]}]}`))
			packageImage, err := buildpackage.NewImage(image)
			h.AssertNil(t, err)
			
			metadata := packageImage.Metadata()
			
			h.AssertEq(t, metadata, buildpackage.Metadata{
				BuildpackInfo: dist.BuildpackInfo{
					ID:      "some.buildpack.id",
					Version: "some.buildpack.version",
				},
				Stacks: []dist.Stack{
					{ ID: "some.stack.id", Mixins: []string{"some-mixin"}},
				},
			})
		})
	})

	when("#SetMetadata", func() {
		it("sets metadata on image label", func() {
			packageImage, err := buildpackage.NewImage(image)
			h.AssertNil(t, err)
			
			h.AssertNil(t, packageImage.SetMetadata(buildpackage.Metadata{
				BuildpackInfo: dist.BuildpackInfo{
					ID:      "some.buildpack.id",
					Version: "some.buildpack.version",
				},
				Stacks: []dist.Stack{
					{ ID: "some.stack.id", Mixins: []string{"some-mixin"}},
				},
			}))
			
			label, err := image.Label("io.buildpacks.buildpackage.metadata")
			h.AssertNil(t, err)
			h.AssertEq(t, label, `{"id":"some.buildpack.id","version":"some.buildpack.version","stacks":[{"ID":"some.stack.id","Mixins":["some-mixin"]}]}`)
		})

		it("updates cached metadata", func() {
			packageImage, err := buildpackage.NewImage(image)
			h.AssertNil(t, err)

			h.AssertNil(t, packageImage.SetMetadata(buildpackage.Metadata{
				BuildpackInfo: dist.BuildpackInfo{
					ID:      "some.buildpack.id",
					Version: "some.buildpack.version",
				},
				Stacks: []dist.Stack{
					{ ID: "some.stack.id", Mixins: []string{"some-mixin"}},
				},
			}))
			
			h.AssertEq(t, packageImage.Metadata(), buildpackage.Metadata{
				BuildpackInfo: dist.BuildpackInfo{
					ID:      "some.buildpack.id",
					Version: "some.buildpack.version",
				},
				Stacks: []dist.Stack{
					{ ID: "some.stack.id", Mixins: []string{"some-mixin"}},
				},
			})
		})
	})
}
