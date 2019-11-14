package builder_test

import (
	"io/ioutil"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Masterminds/semver"
	"github.com/buildpack/imgutil/fakes"
	"github.com/golang/mock/gomock"
	"github.com/heroku/color"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	pubbldr "github.com/buildpack/pack/builder"
	"github.com/buildpack/pack/internal/api"
	"github.com/buildpack/pack/internal/archive"
	"github.com/buildpack/pack/internal/builder"
	"github.com/buildpack/pack/internal/builder/testmocks"
	"github.com/buildpack/pack/internal/dist"
	"github.com/buildpack/pack/internal/stack"
	"github.com/buildpack/pack/logging"
	h "github.com/buildpack/pack/testhelpers"
)

func TestCompat(t *testing.T) {
	color.Disable(true)
	defer func() { color.Disable(false) }()
	spec.Run(t, "Compat", testCompat, spec.Parallel(), spec.Report(report.Terminal{}))
}

func testCompat(t *testing.T, when spec.G, it spec.S) {
	var (
		baseImage      *fakes.Image
		subject        *builder.Builder
		mockController *gomock.Controller
		mockLifecycle  *testmocks.MockLifecycle
		bp1v1          dist.Buildpack
		bp1v2          dist.Buildpack
		bp2v1          dist.Buildpack
		bpOrder        dist.Buildpack
		logger         logging.Logger
	)

	it.Before(func() {
		baseImage = fakes.NewImage("base/image", "", nil)
		mockController = gomock.NewController(t)
		mockLifecycle = testmocks.NewMockLifecycle(mockController)

		mockLifecycle.EXPECT().Open().Return(archive.ReadDirAsTar(
			filepath.Join("testdata", "lifecycle"), ".", 0, 0, -1), nil).AnyTimes()

		bp1v1 = &fakeBuildpack{descriptor: dist.BuildpackDescriptor{
			API: api.MustParse("0.1"),
			Info: dist.BuildpackInfo{
				ID:      "buildpack-1-id",
				Version: "buildpack-1-version-1",
			},
			Stacks: []dist.Stack{{ID: "some.stack.id"}},
		}}
		bp1v2 = &fakeBuildpack{descriptor: dist.BuildpackDescriptor{
			API: api.MustParse("0.1"),
			Info: dist.BuildpackInfo{
				ID:      "buildpack-1-id",
				Version: "buildpack-1-version-2",
			},
			Stacks: []dist.Stack{{ID: "some.stack.id"}},
		}}
		bp2v1 = &fakeBuildpack{descriptor: dist.BuildpackDescriptor{
			API: api.MustParse("0.1"),
			Info: dist.BuildpackInfo{
				ID:      "buildpack-2-id",
				Version: "buildpack-2-version-1",
			},
			Stacks: []dist.Stack{{ID: "some.stack.id"}},
		}}
		bpOrder = &fakeBuildpack{descriptor: dist.BuildpackDescriptor{
			API: api.MustParse("0.1"),
			Info: dist.BuildpackInfo{
				ID:      "order-buildpack-id",
				Version: "order-buildpack-version",
			},
			Order: []dist.OrderEntry{{
				Group: []dist.BuildpackRef{
					{
						BuildpackInfo: bp1v1.Descriptor().Info,
						Optional:      false,
					},
					{
						BuildpackInfo: bp2v1.Descriptor().Info,
						Optional:      false,
					},
				},
			}},
		}}

		h.AssertNil(t, baseImage.SetEnv("CNB_USER_ID", "1234"))
		h.AssertNil(t, baseImage.SetEnv("CNB_GROUP_ID", "4321"))
		h.AssertNil(t, baseImage.SetLabel("io.buildpacks.stack.id", "some.stack.id"))

		logger = logging.New(ioutil.Discard)

		stackImage, err := stack.NewImage(baseImage)
		h.AssertNil(t, err)

		buildImage, err := stack.NewBuildImage(stackImage)
		h.AssertNil(t, err)

		builderImage, err := builder.NewImage(buildImage)
		h.AssertNil(t, err)

		subject, err = builder.FromBuilderImage(builderImage)
		h.AssertNil(t, err)
	})

	it.After(func() {
		baseImage.Cleanup()
		mockController.Finish()
	})

	when("lifecycle < 0.4.0", func() {
		it.Before(func() {
			mockLifecycle.EXPECT().Descriptor().Return(builder.LifecycleDescriptor{
				Info: builder.LifecycleInfo{
					Version: &builder.Version{Version: *semver.MustParse("0.3.0")},
				},
				API: builder.LifecycleAPI{
					PlatformVersion:  api.MustParse("0.1"),
					BuildpackVersion: api.MustParse("0.1"),
				},
			}).AnyTimes()

			subject.SetLifecycle(mockLifecycle)

			subject.AddBuildpack(bp1v1)

			subject.AddBuildpack(bp1v2)

			subject.AddBuildpack(bp2v1)
		})

		it("adds a compat symlink for each buildpack", func() {
			var (
				layerTar string
				err      error
			)

			_, err = subject.Save(logger, baseImage)
			h.AssertNil(t, err)
			h.AssertEq(t, baseImage.IsSaved(), true)

			layerTar, err = baseImage.FindLayerWithPath("/buildpacks/buildpack-1-id/buildpack-1-version-1")
			h.AssertNil(t, err)

			h.AssertOnTarEntry(t, layerTar, "/buildpacks/buildpack-1-id",
				h.HasModTime(archive.NormalizedDateTime),
			)

			h.AssertOnTarEntry(t, layerTar, "/buildpacks/buildpack-1-id/buildpack-1-version-1",
				h.SymlinksTo("/cnb/buildpacks/buildpack-1-id/buildpack-1-version-1"),
				h.HasModTime(archive.NormalizedDateTime),
			)

			layerTar, err = baseImage.FindLayerWithPath("/buildpacks/buildpack-2-id/buildpack-2-version-1")
			h.AssertNil(t, err)

			h.AssertOnTarEntry(t, layerTar, "/buildpacks/buildpack-2-id",
				h.HasModTime(archive.NormalizedDateTime),
			)

			h.AssertOnTarEntry(t, layerTar, "/buildpacks/buildpack-2-id/buildpack-2-version-1",
				h.SymlinksTo("/cnb/buildpacks/buildpack-2-id/buildpack-2-version-1"),
				h.HasModTime(archive.NormalizedDateTime),
			)
		})

		it("adds latest buildpack symlinks", func() {
			_, err := subject.Save(logger, baseImage)
			h.AssertNil(t, err)
			h.AssertEq(t, baseImage.IsSaved(), true)

			layerTar, err := baseImage.FindLayerWithPath("/buildpacks/buildpack-2-id/buildpack-2-version-1")
			h.AssertNil(t, err)

			h.AssertOnTarEntry(t, layerTar, "/buildpacks/buildpack-2-id/latest",
				h.SymlinksTo("/cnb/buildpacks/buildpack-2-id/buildpack-2-version-1"),
			)
		})

		it("creates the compat buildpacks dir", func() {
			_, err := subject.Save(logger, baseImage)
			h.AssertNil(t, err)
			h.AssertEq(t, baseImage.IsSaved(), true)

			layerTar, err := baseImage.FindLayerWithPath("/buildpacks")
			h.AssertNil(t, err)
			h.AssertOnTarEntry(t, layerTar, "/buildpacks",
				h.IsDirectory(),
				h.HasOwnerAndGroup(0, 0),
				h.HasFileMode(0755),
				h.HasModTime(archive.NormalizedDateTime),
			)
		})

		when("#SetStack", func() {
			it.Before(func() {
				subject.SetStack(pubbldr.StackConfig{
					RunImage:        "some/run",
					RunImageMirrors: []string{"some/mirror", "other/mirror"},
				})
				_, err := subject.Save(logger, baseImage)
				h.AssertNil(t, err)
				h.AssertEq(t, baseImage.IsSaved(), true)
			})

			it("adds a compat stack.toml to the image", func() {
				layerTar, err := baseImage.FindLayerWithPath("/buildpacks/stack.toml")
				h.AssertNil(t, err)
				h.AssertOnTarEntry(t, layerTar, "/buildpacks/stack.toml",
					h.ContentEquals(`[run-image]
  image = "some/run"
  mirrors = ["some/mirror", "other/mirror"]
`),
					h.HasModTime(archive.NormalizedDateTime),
				)
			})
		})

		when("buildpacks exist in the image", func() {
			it.Before(func() {
				subject.AddBuildpack(bp1v1)
				subject.AddBuildpack(bp1v2)
				subject.AddBuildpack(bp2v1)
				subject.AddBuildpack(bpOrder)

				subject.SetOrder(dist.Order{
					{Group: []dist.BuildpackRef{
						{
							BuildpackInfo: dist.BuildpackInfo{
								ID:      "buildpack-1-id",
								Version: "buildpack-1-version-1",
							},
							Optional: false,
						},
						{
							BuildpackInfo: dist.BuildpackInfo{
								ID:      "buildpack-2-id",
								Version: "buildpack-2-version-1",
							},
							Optional: true,
						},
					}},
				})

				_, err := subject.Save(logger, baseImage)
				h.AssertNil(t, err)
				h.AssertEq(t, baseImage.IsSaved(), true)
			})
		})
	})

	when("lifecycle >= 0.4.0", func() {
		it.Before(func() {
			mockLifecycle.EXPECT().Descriptor().Return(builder.LifecycleDescriptor{
				Info: builder.LifecycleInfo{
					Version: &builder.Version{Version: *semver.MustParse("0.4.0")},
				},
				API: builder.LifecycleAPI{
					PlatformVersion:  api.MustParse("0.2"),
					BuildpackVersion: api.MustParse("0.2"),
				},
			}).AnyTimes()

			subject.SetLifecycle(mockLifecycle)

			subject.AddBuildpack(updateFakeAPIVersion(bp1v1, api.MustParse("0.2")))
			subject.AddBuildpack(updateFakeAPIVersion(bp1v2, api.MustParse("0.2")))
			subject.AddBuildpack(updateFakeAPIVersion(bp2v1, api.MustParse("0.2")))
			subject.AddBuildpack(updateFakeAPIVersion(bpOrder, api.MustParse("0.2")))

			subject.SetOrder(dist.Order{
				{Group: []dist.BuildpackRef{
					{
						BuildpackInfo: dist.BuildpackInfo{
							ID:      "buildpack-1-id",
							Version: "buildpack-1-version-1",
						},
						Optional: false,
					},
					{
						BuildpackInfo: dist.BuildpackInfo{
							ID:      "buildpack-2-id",
							Version: "buildpack-2-version-1",
						},
						Optional: true,
					},
				}},
			})

			_, err := subject.Save(logger, baseImage)
			h.AssertNil(t, err)
			h.AssertEq(t, baseImage.IsSaved(), true)
		})

		it("doesn't add the latest buildpack symlink", func() {
			layerTar, err := baseImage.FindLayerWithPath("/buildpacks/buildpack-1-id/buildpack-1-version-1")
			h.AssertNil(t, err)

			headers, err := h.ListTarContents(layerTar)
			h.AssertNil(t, err)
			for _, header := range headers {
				if strings.Contains(header.Name, "latest") {
					t.Fatalf("found an unexpected latest entry %s", header.Name)
				}
			}
		})
	})

	when("always", func() {
		it.Before(func() {
			mockLifecycle.EXPECT().Descriptor().Return(builder.LifecycleDescriptor{
				Info: builder.LifecycleInfo{
					Version: &builder.Version{Version: *semver.MustParse("0.4.0")},
				},
				API: builder.LifecycleAPI{
					PlatformVersion:  api.MustParse("0.2"),
					BuildpackVersion: api.MustParse("0.2"),
				},
			}).AnyTimes()

			subject.SetLifecycle(mockLifecycle)
		})

		it("should create a compat lifecycle symlink", func() {
			_, err := subject.Save(logger, baseImage)
			h.AssertNil(t, err)
			h.AssertEq(t, baseImage.IsSaved(), true)

			layerTar, err := baseImage.FindLayerWithPath("/lifecycle")
			h.AssertNil(t, err)
			h.AssertOnTarEntry(t, layerTar, "/lifecycle",
				h.SymlinksTo("/cnb/lifecycle"),
				h.HasModTime(archive.NormalizedDateTime),
			)
		})
	})
}

func updateFakeAPIVersion(buildpack dist.Buildpack, version *api.Version) dist.Buildpack {
	return &fakeBuildpack{
		descriptor: dist.BuildpackDescriptor{
			API:    version,
			Info:   buildpack.Descriptor().Info,
			Stacks: buildpack.Descriptor().Stacks,
			Order:  buildpack.Descriptor().Order,
		},
	}
}
