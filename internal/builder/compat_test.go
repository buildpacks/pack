package builder_test

import (
	"io/ioutil"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Masterminds/semver"
	"github.com/buildpacks/imgutil/fakes"
	"github.com/golang/mock/gomock"
	"github.com/heroku/color"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	pubbldr "github.com/buildpacks/pack/builder"
	"github.com/buildpacks/pack/internal/api"
	"github.com/buildpacks/pack/internal/archive"
	"github.com/buildpacks/pack/internal/builder"
	"github.com/buildpacks/pack/internal/builder/testmocks"
	"github.com/buildpacks/pack/internal/dist"
	ifakes "github.com/buildpacks/pack/internal/fakes"
	"github.com/buildpacks/pack/logging"
	h "github.com/buildpacks/pack/testhelpers"
)

func TestCompat(t *testing.T) {
	color.Disable(true)
	defer color.Disable(false)
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

		var err error
		bp1v1, err = ifakes.NewFakeBuildpack(dist.BuildpackDescriptor{
			API: api.MustParse("0.1"),
			Info: dist.BuildpackInfo{
				ID:      "buildpack-1-id",
				Version: "buildpack-1-version-1",
			},
			Stacks: []dist.Stack{{ID: "some.stack.id"}},
		}, 0644)
		h.AssertNil(t, err)

		bp1v2, err = ifakes.NewFakeBuildpack(dist.BuildpackDescriptor{
			API: api.MustParse("0.1"),
			Info: dist.BuildpackInfo{
				ID:      "buildpack-1-id",
				Version: "buildpack-1-version-2",
			},
			Stacks: []dist.Stack{{ID: "some.stack.id"}},
		}, 0644)
		h.AssertNil(t, err)

		bp2v1, err = ifakes.NewFakeBuildpack(dist.BuildpackDescriptor{
			API: api.MustParse("0.1"),
			Info: dist.BuildpackInfo{
				ID:      "buildpack-2-id",
				Version: "buildpack-2-version-1",
			},
			Stacks: []dist.Stack{{ID: "some.stack.id"}},
		}, 0644)
		h.AssertNil(t, err)

		bpOrder, err = ifakes.NewFakeBuildpack(dist.BuildpackDescriptor{
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
		}, 0644)
		h.AssertNil(t, err)

		h.AssertNil(t, baseImage.SetEnv("CNB_USER_ID", "1234"))
		h.AssertNil(t, baseImage.SetEnv("CNB_GROUP_ID", "4321"))
		h.AssertNil(t, baseImage.SetLabel("io.buildpacks.stack.id", "some.stack.id"))

		logger = logging.New(ioutil.Discard)

		subject, err = builder.New(baseImage, "some/builder")
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

			h.AssertNil(t, subject.SetLifecycle(mockLifecycle))

			subject.AddBuildpack(bp1v1)

			subject.AddBuildpack(bp1v2)

			subject.AddBuildpack(bp2v1)
		})

		it("adds a compat symlink for each buildpack", func() {
			var (
				layerTar string
				err      error
			)

			h.AssertNil(t, subject.Save(logger))
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
			h.AssertNil(t, subject.Save(logger))
			h.AssertEq(t, baseImage.IsSaved(), true)

			layerTar, err := baseImage.FindLayerWithPath("/buildpacks/buildpack-2-id/buildpack-2-version-1")
			h.AssertNil(t, err)

			h.AssertOnTarEntry(t, layerTar, "/buildpacks/buildpack-2-id/latest",
				h.SymlinksTo("/cnb/buildpacks/buildpack-2-id/buildpack-2-version-1"),
			)
		})

		it("creates the compat buildpacks dir", func() {
			h.AssertNil(t, subject.Save(logger))
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
				h.AssertNil(t, subject.Save(logger))
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

				h.AssertNil(t, subject.Save(logger))
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

			h.AssertNil(t, subject.SetLifecycle(mockLifecycle))

			for _, bp := range []dist.Buildpack{bp1v1, bp1v2, bp2v1, bpOrder} {
				updated, err := updateFakeAPIVersion(bp, api.MustParse("0.2"))
				h.AssertNil(t, err)
				subject.AddBuildpack(updated)
			}

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

			h.AssertNil(t, subject.Save(logger))
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

			h.AssertNil(t, subject.SetLifecycle(mockLifecycle))
		})

		it("should create a compat lifecycle symlink", func() {
			h.AssertNil(t, subject.Save(logger))
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

func updateFakeAPIVersion(buildpack dist.Buildpack, version *api.Version) (dist.Buildpack, error) {
	return ifakes.NewFakeBuildpack(dist.BuildpackDescriptor{
		API:    version,
		Info:   buildpack.Descriptor().Info,
		Stacks: buildpack.Descriptor().Stacks,
		Order:  buildpack.Descriptor().Order,
	}, 0644)
}
