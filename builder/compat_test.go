package builder_test

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Masterminds/semver"
	"github.com/buildpack/imgutil/fakes"
	"github.com/fatih/color"
	"github.com/golang/mock/gomock"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	"github.com/buildpack/pack/api"
	"github.com/buildpack/pack/builder"
	"github.com/buildpack/pack/builder/testmocks"
	"github.com/buildpack/pack/internal/archive"
	ifakes "github.com/buildpack/pack/internal/fakes"
	h "github.com/buildpack/pack/testhelpers"
)

func TestCompat(t *testing.T) {
	color.NoColor = true
	spec.Run(t, "Compat", testCompat, spec.Parallel(), spec.Report(report.Terminal{}))
}

func testCompat(t *testing.T, when spec.G, it spec.S) {
	var (
		baseImage      *fakes.Image
		subject        *builder.Builder
		mockController *gomock.Controller
		mockLifecycle  *testmocks.MockLifecycle
		bp1v1          builder.Buildpack
		bp1v2          builder.Buildpack
		bp2v1          builder.Buildpack
		bpOrder        builder.Buildpack
	)

	it.Before(func() {
		baseImage = fakes.NewImage("base/image", "", "")
		mockController = gomock.NewController(t)
		mockLifecycle = testmocks.NewMockLifecycle(mockController)

		mockLifecycle.EXPECT().Open().Return(archive.ReadDirAsTar(
			filepath.Join("testdata", "lifecycle"), ".", 0, 0, -1), nil).AnyTimes()

		bp1v1 = &fakeBuildpack{descriptor: builder.BuildpackDescriptor{
			API: api.MustParse("0.1"),
			Info: builder.BuildpackInfo{
				ID:      "buildpack-1-id",
				Version: "buildpack-1-version-1",
			},
			Stacks: []builder.Stack{{ID: "some.stack.id"}},
		}}
		bp1v2 = &fakeBuildpack{descriptor: builder.BuildpackDescriptor{
			API: api.MustParse("0.1"),
			Info: builder.BuildpackInfo{
				ID:      "buildpack-1-id",
				Version: "buildpack-1-version-2",
			},
			Stacks: []builder.Stack{{ID: "some.stack.id"}},
		}}
		bp2v1 = &fakeBuildpack{descriptor: builder.BuildpackDescriptor{
			API: api.MustParse("0.1"),
			Info: builder.BuildpackInfo{
				ID:      "buildpack-2-id",
				Version: "buildpack-2-version-1",
			},
			Stacks: []builder.Stack{{ID: "some.stack.id"}},
		}}
		bpOrder = &fakeBuildpack{descriptor: builder.BuildpackDescriptor{
			API: api.MustParse("0.1"),
			Info: builder.BuildpackInfo{
				ID:      "order-buildpack-id",
				Version: "order-buildpack-version",
			},
			Order: []builder.OrderEntry{{
				Group: []builder.BuildpackRef{
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

		var err error
		subject, err = builder.New(ifakes.NewFakeLogger(ioutil.Discard), baseImage, "some/builder")
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

			h.AssertNil(t, subject.Save())
			h.AssertEq(t, baseImage.IsSaved(), true)

			layerTar, err = baseImage.FindLayerWithPath("/buildpacks/buildpack-1-id/buildpack-1-version-1")
			h.AssertNil(t, err)

			h.AssertOnTarEntry(t, layerTar, "/buildpacks/buildpack-1-id/buildpack-1-version-1",
				h.SymlinksTo("/cnb/buildpacks/buildpack-1-id/buildpack-1-version-1"),
			)

			layerTar, err = baseImage.FindLayerWithPath("/buildpacks/buildpack-2-id/buildpack-2-version-1")
			h.AssertNil(t, err)

			h.AssertOnTarEntry(t, layerTar, "/buildpacks/buildpack-2-id/buildpack-2-version-1",
				h.SymlinksTo("/cnb/buildpacks/buildpack-2-id/buildpack-2-version-1"),
			)
		})

		it("adds latest buildpack symlinks", func() {
			h.AssertNil(t, subject.Save())
			h.AssertEq(t, baseImage.IsSaved(), true)

			layerTar, err := baseImage.FindLayerWithPath("/buildpacks/buildpack-2-id/buildpack-2-version-1")
			h.AssertNil(t, err)

			h.AssertOnTarEntry(t, layerTar, "/buildpacks/buildpack-2-id/latest",
				h.SymlinksTo("/cnb/buildpacks/buildpack-2-id/buildpack-2-version-1"),
			)
		})

		it("creates the compat buildpacks dir", func() {
			h.AssertNil(t, subject.Save())
			h.AssertEq(t, baseImage.IsSaved(), true)

			layerTar, err := baseImage.FindLayerWithPath("/buildpacks")
			h.AssertNil(t, err)
			h.AssertOnTarEntry(t, layerTar, "/buildpacks",
				h.IsDirectory(),
				h.HasOwnerAndGroup(0, 0),
				h.HasFileMode(0755),
			)
		})

		when("#SetStackInfo", func() {
			it.Before(func() {
				subject.SetStackInfo(builder.StackConfig{
					RunImage:        "some/run",
					RunImageMirrors: []string{"some/mirror", "other/mirror"},
				})
				h.AssertNil(t, subject.Save())
				h.AssertEq(t, baseImage.IsSaved(), true)
			})

			it("adds a compat stack.toml to the image", func() {
				layerTar, err := baseImage.FindLayerWithPath("/buildpacks/stack.toml")
				h.AssertNil(t, err)
				h.AssertOnTarEntry(t, layerTar, "/buildpacks/stack.toml", h.ContentEquals(`[run-image]
  image = "some/run"
  mirrors = ["some/mirror", "other/mirror"]
`))
			})
		})

		when("buildpacks exist in the image", func() {
			it.Before(func() {
				subject.AddBuildpack(bp1v1)
				subject.AddBuildpack(bp1v2)
				subject.AddBuildpack(bp2v1)
				subject.AddBuildpack(bpOrder)

				subject.SetOrder(builder.Order{
					{Group: []builder.BuildpackRef{
						{
							BuildpackInfo: builder.BuildpackInfo{
								ID:      "buildpack-1-id",
								Version: "buildpack-1-version-1",
							},
							Optional: false,
						},
						{
							BuildpackInfo: builder.BuildpackInfo{
								ID:      "buildpack-2-id",
								Version: "buildpack-2-version-1",
							},
							Optional: true,
						},
					}},
				})

				h.AssertNil(t, subject.Save())
				h.AssertEq(t, baseImage.IsSaved(), true)
			})

			it("adds a compat order.toml to the image", func() {
				layerTar, err := baseImage.FindLayerWithPath("/buildpacks/order.toml")
				h.AssertNil(t, err)

				h.AssertOnTarEntry(t, layerTar, "/buildpacks/order.toml", h.ContentEquals(`[[groups]]

  [[groups.buildpacks]]
    id = "buildpack-1-id"
    version = "buildpack-1-version-1"

  [[groups.buildpacks]]
    id = "buildpack-2-id"
    version = "buildpack-2-version-1"
    optional = true
`))
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

			subject.AddBuildpack(updateFakeAPIVersion(bp1v1, api.MustParse("0.2")))
			subject.AddBuildpack(updateFakeAPIVersion(bp1v2, api.MustParse("0.2")))
			subject.AddBuildpack(updateFakeAPIVersion(bp2v1, api.MustParse("0.2")))
			subject.AddBuildpack(updateFakeAPIVersion(bpOrder, api.MustParse("0.2")))

			subject.SetOrder(builder.Order{
				{Group: []builder.BuildpackRef{
					{
						BuildpackInfo: builder.BuildpackInfo{
							ID:      "buildpack-1-id",
							Version: "buildpack-1-version-1",
						},
						Optional: false,
					},
					{
						BuildpackInfo: builder.BuildpackInfo{
							ID:      "buildpack-2-id",
							Version: "buildpack-2-version-1",
						},
						Optional: true,
					},
				}},
			})

			h.AssertNil(t, subject.Save())
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

		it("adds a compat order.toml to the image", func() {
			layerTar, err := baseImage.FindLayerWithPath("/buildpacks/order.toml")
			h.AssertNil(t, err)

			h.AssertOnTarEntry(t, layerTar, "/buildpacks/order.toml", h.ContentEquals(`[[order]]

  [[order.group]]
    id = "buildpack-1-id"
    version = "buildpack-1-version-1"

  [[order.group]]
    id = "buildpack-2-id"
    version = "buildpack-2-version-1"
    optional = true
`))
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
			h.AssertNil(t, subject.Save())
			h.AssertEq(t, baseImage.IsSaved(), true)

			layerTar, err := baseImage.FindLayerWithPath("/lifecycle")
			h.AssertNil(t, err)
			h.AssertOnTarEntry(t, layerTar, "/lifecycle", h.SymlinksTo("/cnb/lifecycle"))
		})

		it("does not overwrite the compat order when SetOrder has not been called", func() {
			tmpDir, err := ioutil.TempDir("", "")
			h.AssertNil(t, err)
			defer os.RemoveAll(tmpDir)

			layerFile := filepath.Join(tmpDir, "order.tar")

			err = archive.CreateSingleFileTar(layerFile, "/buildpacks/order.toml", "some content")
			h.AssertNil(t, err)

			h.AssertNil(t, baseImage.AddLayer(layerFile))
			_, err = baseImage.Save()
			h.AssertNil(t, err)

			h.AssertNil(t, subject.Save())
			h.AssertEq(t, baseImage.IsSaved(), true)

			layerTar, err := baseImage.FindLayerWithPath("/buildpacks/order.toml")
			h.AssertNil(t, err)
			h.AssertOnTarEntry(t, layerTar, "/buildpacks/order.toml", h.ContentEquals("some content"))
		})
	})
}

func updateFakeAPIVersion(buildpack builder.Buildpack, version *api.Version) builder.Buildpack {
	return &fakeBuildpack{
		descriptor: builder.BuildpackDescriptor{
			API:    version,
			Info:   buildpack.Descriptor().Info,
			Stacks: buildpack.Descriptor().Stacks,
			Order:  buildpack.Descriptor().Order,
		},
	}
}
