package builder_test

import (
	"errors"
	"github.com/buildpack/pack/builder"
	"github.com/buildpack/pack/config"
	"github.com/buildpack/pack/mocks"
	h "github.com/buildpack/pack/testhelpers"
	"github.com/fatih/color"
	"github.com/golang/mock/gomock"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"
	"testing"
)

func TestBuilder(t *testing.T) {
	color.NoColor = true
	spec.Run(t, "Builder", testBuilder, spec.Parallel(), spec.Report(report.Terminal{}))
}

func testBuilder(t *testing.T, when spec.G, it spec.S) {
	var (
		mockController *gomock.Controller
		mockImage      *mocks.MockImage
		cfg            *config.Config
		bldr           *builder.Builder
	)

	it.Before(func() {
		mockController = gomock.NewController(t)
		mockImage = mocks.NewMockImage(mockController)
		mockImage.EXPECT().Name().Return("some/builder")
		cfg = &config.Config{}
		bldr = builder.NewBuilder(mockImage, cfg)
	})

	when("#GetStack", func() {
		when("error getting stack label", func() {
			it.Before(func() {
				mockImage.EXPECT().Label("io.buildpacks.stack.id").Return("", errors.New("some error"))
			})

			it("returns an error", func() {
				_, err := bldr.GetStack()
				h.AssertError(t, err, "failed to find stack label for builder 'some/builder'")
			})
		})

		when("stack label is empty", func() {
			it.Before(func() {
				mockImage.EXPECT().Label("io.buildpacks.stack.id").Return("", nil)
			})

			it("returns an error", func() {
				_, err := bldr.GetStack()
				h.AssertError(t, err, "builder 'some/builder' missing label 'io.buildpacks.stack.id' -- try recreating builder")
			})
		})
	})

	when("#GetMetadata", func() {
		when("error getting metadata label", func() {
			it.Before(func() {
				mockImage.EXPECT().Label("io.buildpacks.builder.metadata").Return("", errors.New("some error"))
			})

			it("returns an error", func() {
				_, err := bldr.GetMetadata()
				h.AssertError(t, err, "failed to find run images for builder 'some/builder'")
			})
		})

		when("metadata label is empty", func() {
			it.Before(func() {
				mockImage.EXPECT().Label("io.buildpacks.builder.metadata").Return("", nil)
			})

			it("returns an error", func() {
				_, err := bldr.GetMetadata()
				h.AssertError(t, err, "builder 'some/builder' missing label 'io.buildpacks.builder.metadata' -- try recreating builder")
			})
		})

		when("metadata label is not parsable", func() {
			it.Before(func() {
				mockImage.EXPECT().Label("io.buildpacks.builder.metadata").Return("junk", nil)
			})

			it("returns an error", func() {
				_, err := bldr.GetMetadata()
				h.AssertError(t, err, "failed to parse metadata for builder 'some/builder'")
			})
		})
	})

	when("#GetLocalRunImageMirrors", func() {
		when("run image exists in config", func() {
			it.Before(func() {
				mockImage.EXPECT().Label("io.buildpacks.builder.metadata").Return(`{
 "runImage": {
   "image": "some/run-image",
   "mirrors": []
 }
}`, nil)
				cfg.RunImages = []config.RunImage{{Image: "some/run-image", Mirrors: []string{"a", "b"}}}
			})

			it("returns the local mirrors", func() {
				localMirrors, err := bldr.GetLocalRunImageMirrors()
				h.AssertNil(t, err)
				h.AssertSliceContains(t, localMirrors, "a")
				h.AssertSliceContains(t, localMirrors, "b")
			})
		})

		when("run image does not exist in config", func() {
			it.Before(func() {
				mockImage.EXPECT().Label("io.buildpacks.builder.metadata").Return(`{
 "runImage": {
   "image": "some/other-run-image",
   "mirrors": []
 }
}`, nil)
				cfg.RunImages = []config.RunImage{{Image: "some/run-image", Mirrors: []string{"a", "b"}}}
			})

			it("returns an empty slice", func() {
				localMirrors, err := bldr.GetLocalRunImageMirrors()
				h.AssertNil(t, err)
				h.AssertEq(t, len(localMirrors), 0)
			})
		})
	})
}
