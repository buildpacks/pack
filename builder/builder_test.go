package builder_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path"
	"path/filepath"
	"testing"

	"github.com/Masterminds/semver"
	"github.com/buildpack/imgutil"
	"github.com/buildpack/imgutil/fakes"
	"github.com/golang/mock/gomock"
	"github.com/heroku/color"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	"github.com/buildpack/pack/api"
	"github.com/buildpack/pack/builder"
	"github.com/buildpack/pack/builder/testmocks"
	"github.com/buildpack/pack/dist"
	"github.com/buildpack/pack/internal/archive"
	ifakes "github.com/buildpack/pack/internal/fakes"
	"github.com/buildpack/pack/logging"
	h "github.com/buildpack/pack/testhelpers"
)

func TestBuilder(t *testing.T) {
	color.Disable(true)
	defer func() { color.Disable(false) }()
	spec.Run(t, "Builder", testBuilder, spec.Parallel(), spec.Report(report.Terminal{}))
}

func testBuilder(t *testing.T, when spec.G, it spec.S) {
	var (
		baseImage      *fakes.Image
		subject        *builder.Builder
		mockController *gomock.Controller
		mockLifecycle  *testmocks.MockLifecycle
		bp1v1          dist.Buildpack
		bp1v2          dist.Buildpack
		bp2v1          dist.Buildpack
		bpOrder        dist.Buildpack
		buf            bytes.Buffer
		logger         logging.Logger
	)

	it.Before(func() {
		logger = ifakes.NewFakeLogger(&buf)
		baseImage = fakes.NewImage("base/image", "", "")
		mockController = gomock.NewController(t)
		mockLifecycle = testmocks.NewMockLifecycle(mockController)
		mockLifecycle.EXPECT().Open().Return(archive.ReadDirAsTar(
			filepath.Join("testdata", "lifecycle"), ".", 0, 0, 0755), nil).AnyTimes()
		mockLifecycle.EXPECT().Descriptor().Return(builder.LifecycleDescriptor{
			Info: builder.LifecycleInfo{
				Version: &builder.Version{Version: *semver.MustParse("1.2.3")},
			},
			API: builder.LifecycleAPI{
				PlatformVersion:  api.MustParse("2.2"),
				BuildpackVersion: api.MustParse("0.2"),
			},
		}).AnyTimes()

		bp1v1 = &fakeBuildpack{descriptor: dist.BuildpackDescriptor{
			API: api.MustParse("0.2"),
			Info: dist.BuildpackInfo{
				ID:      "buildpack-1-id",
				Version: "buildpack-1-version-1",
			},
			Stacks: []dist.Stack{{ID: "some.stack.id"}},
		}}
		bp1v2 = &fakeBuildpack{descriptor: dist.BuildpackDescriptor{
			API: api.MustParse("0.2"),
			Info: dist.BuildpackInfo{
				ID:      "buildpack-1-id",
				Version: "buildpack-1-version-2",
			},
			Stacks: []dist.Stack{{ID: "some.stack.id"}},
		}}
		bp2v1 = &fakeBuildpack{descriptor: dist.BuildpackDescriptor{
			API: api.MustParse("0.2"),
			Info: dist.BuildpackInfo{
				ID:      "buildpack-2-id",
				Version: "buildpack-2-version-1",
			},
			Stacks: []dist.Stack{{ID: "some.stack.id"}},
		}}
		bpOrder = &fakeBuildpack{descriptor: dist.BuildpackDescriptor{
			API: api.MustParse("0.2"),
			Info: dist.BuildpackInfo{
				ID:      "order-buildpack-id",
				Version: "order-buildpack-version",
			},
			Order: []dist.OrderEntry{{
				Group: []dist.BuildpackRef{
					{
						BuildpackInfo: bp1v1.Descriptor().Info,
						Optional:      true,
					},
					{
						BuildpackInfo: bp2v1.Descriptor().Info,
						Optional:      false,
					},
				},
			}},
		}}
	})

	it.After(func() {
		baseImage.Cleanup()
		mockController.Finish()
	})

	when("the base image is not valid", func() {
		when("#New", func() {
			when("missing CNB_USER_ID", func() {
				it("returns an error", func() {
					_, err := builder.New(baseImage, "some/builder")
					h.AssertError(t, err, "image 'base/image' missing required env var 'CNB_USER_ID'")
				})
			})

			when("missing CNB_GROUP_ID", func() {
				it.Before(func() {
					h.AssertNil(t, baseImage.SetEnv("CNB_USER_ID", "1234"))
				})

				it("returns an error", func() {
					_, err := builder.New(baseImage, "some/builder")
					h.AssertError(t, err, "image 'base/image' missing required env var 'CNB_GROUP_ID'")
				})
			})

			when("CNB_USER_ID is not an int", func() {
				it.Before(func() {
					h.AssertNil(t, baseImage.SetEnv("CNB_USER_ID", "not an int"))
					h.AssertNil(t, baseImage.SetEnv("CNB_GROUP_ID", "4321"))
				})

				it("returns an error", func() {
					_, err := builder.New(baseImage, "some/builder")
					h.AssertError(t, err, "failed to parse 'CNB_USER_ID', value 'not an int' should be an integer")
				})
			})

			when("CNB_GROUP_ID is not an int", func() {
				it.Before(func() {
					h.AssertNil(t, baseImage.SetEnv("CNB_USER_ID", "1234"))
					h.AssertNil(t, baseImage.SetEnv("CNB_GROUP_ID", "not an int"))
				})

				it("returns an error", func() {
					_, err := builder.New(baseImage, "some/builder")
					h.AssertError(t, err, "failed to parse 'CNB_GROUP_ID', value 'not an int' should be an integer")
				})
			})

			when("missing stack id label", func() {
				it.Before(func() {
					h.AssertNil(t, baseImage.SetEnv("CNB_USER_ID", "1234"))
					h.AssertNil(t, baseImage.SetEnv("CNB_GROUP_ID", "4321"))
				})

				it("returns an error", func() {
					_, err := builder.New(baseImage, "some/builder")
					h.AssertError(t, err, "image 'base/image' missing label 'io.buildpacks.stack.id'")
				})
			})
		})
	})

	when("the base image is a valid build image", func() {
		it.Before(func() {
			var err error
			h.AssertNil(t, baseImage.SetEnv("CNB_USER_ID", "1234"))
			h.AssertNil(t, baseImage.SetEnv("CNB_GROUP_ID", "4321"))
			h.AssertNil(t, baseImage.SetLabel("io.buildpacks.stack.id", "some.stack.id"))
			subject, err = builder.New(baseImage, "some/builder")
			h.AssertNil(t, err)

			h.AssertNil(t, subject.SetLifecycle(mockLifecycle))
		})

		it.After(func() {
			baseImage.Cleanup()
		})

		when("#Save", func() {
			it("creates a builder from the image and renames it", func() {
				h.AssertNil(t, subject.Save(logger))
				h.AssertEq(t, baseImage.IsSaved(), true)
				h.AssertEq(t, baseImage.Name(), "some/builder")
			})

			it("adds creator metadata", func() {
				h.AssertNil(t, subject.Save(logger))
				h.AssertEq(t, baseImage.IsSaved(), true)

				label, err := baseImage.Label("io.buildpacks.builder.metadata")
				h.AssertNil(t, err)

				var metadata builder.Metadata
				h.AssertNil(t, json.Unmarshal([]byte(label), &metadata))

				h.AssertEq(t, metadata.CreatedBy.Name, "Pack CLI")
				h.AssertEq(t, metadata.CreatedBy.Version, "0.0.0")
			})

			it("creates the workspace dir with CNB user and group", func() {
				h.AssertNil(t, subject.Save(logger))
				h.AssertEq(t, baseImage.IsSaved(), true)

				layerTar, err := baseImage.FindLayerWithPath("/workspace")
				h.AssertNil(t, err)
				h.AssertOnTarEntry(t, layerTar, "/workspace",
					h.IsDirectory(),
					h.HasFileMode(0755),
					h.HasOwnerAndGroup(1234, 4321),
					h.HasModTime(archive.NormalizedDateTime),
				)
			})

			it("creates the layers dir with CNB user and group", func() {
				h.AssertNil(t, subject.Save(logger))
				h.AssertEq(t, baseImage.IsSaved(), true)

				layerTar, err := baseImage.FindLayerWithPath("/layers")
				h.AssertNil(t, err)
				h.AssertOnTarEntry(t, layerTar, "/layers",
					h.IsDirectory(),
					h.HasOwnerAndGroup(1234, 4321),
					h.HasFileMode(0755),
					h.HasModTime(archive.NormalizedDateTime),
				)
			})

			it("creates the cnb dir", func() {
				h.AssertNil(t, subject.Save(logger))
				h.AssertEq(t, baseImage.IsSaved(), true)

				layerTar, err := baseImage.FindLayerWithPath("/cnb")
				h.AssertNil(t, err)
				h.AssertOnTarEntry(t, layerTar, "/cnb",
					h.IsDirectory(),
					h.HasOwnerAndGroup(0, 0),
					h.HasFileMode(0755),
					h.HasModTime(archive.NormalizedDateTime),
				)
			})

			it("creates the buildpacks dir", func() {
				h.AssertNil(t, subject.Save(logger))
				h.AssertEq(t, baseImage.IsSaved(), true)

				layerTar, err := baseImage.FindLayerWithPath("/cnb/buildpacks")
				h.AssertNil(t, err)
				h.AssertOnTarEntry(t, layerTar, "/cnb/buildpacks",
					h.IsDirectory(),
					h.HasOwnerAndGroup(0, 0),
					h.HasFileMode(0755),
					h.HasModTime(archive.NormalizedDateTime),
				)
			})

			it("creates the platform dir", func() {
				h.AssertNil(t, subject.Save(logger))
				h.AssertEq(t, baseImage.IsSaved(), true)

				layerTar, err := baseImage.FindLayerWithPath("/platform")
				h.AssertNil(t, err)
				h.AssertOnTarEntry(t, layerTar, "/platform",
					h.IsDirectory(),
					h.HasOwnerAndGroup(0, 0),
					h.HasFileMode(0755),
					h.HasModTime(archive.NormalizedDateTime),
				)
				h.AssertOnTarEntry(t, layerTar, "/platform/env",
					h.IsDirectory(),
					h.HasOwnerAndGroup(0, 0),
					h.HasFileMode(0755),
					h.HasModTime(archive.NormalizedDateTime),
				)
			})

			it("sets the working dir to the layers dir", func() {
				h.AssertNil(t, subject.Save(logger))
				h.AssertEq(t, baseImage.IsSaved(), true)

				h.AssertEq(t, baseImage.WorkingDir(), "/layers")
			})

			it("does not overwrite the order layer when SetOrder has not been called", func() {
				tmpDir, err := ioutil.TempDir("", "")
				h.AssertNil(t, err)
				defer os.RemoveAll(tmpDir)

				layerFile := filepath.Join(tmpDir, "order.tar")
				f, err := os.Create(layerFile)
				h.AssertNil(t, err)
				defer f.Close()

				err = archive.CreateSingleFileTar(f.Name(), "/cnb/order.toml", "some content")
				h.AssertNil(t, err)

				h.AssertNil(t, baseImage.AddLayer(layerFile))
				baseImage.Save()

				h.AssertNil(t, subject.Save(logger))
				h.AssertEq(t, baseImage.IsSaved(), true)

				layerTar, err := baseImage.FindLayerWithPath("/cnb/order.toml")
				h.AssertNil(t, err)
				h.AssertOnTarEntry(t, layerTar, "/cnb/order.toml", h.ContentEquals("some content"))
			})

			when("validating order", func() {
				it.Before(func() {
					h.AssertNil(t, subject.SetLifecycle(mockLifecycle))
				})

				when("has single buildpack", func() {
					it.Before(func() {
						subject.AddBuildpack(bp1v1)
					})

					it("should resolve unset version (to legacy label and order.toml)", func() {
						subject.SetOrder(dist.Order{{
							Group: []dist.BuildpackRef{
								{BuildpackInfo: dist.BuildpackInfo{ID: bp1v1.Descriptor().Info.ID}}},
						}})

						err := subject.Save(logger)
						h.AssertNil(t, err)

						layerTar, err := baseImage.FindLayerWithPath("/cnb/order.toml")
						h.AssertNil(t, err)
						h.AssertOnTarEntry(t, layerTar, "/cnb/order.toml", h.ContentEquals(`[[order]]

  [[order.group]]
    id = "buildpack-1-id"
    version = "buildpack-1-version-1"
`))
					})

					when("order points to missing buildpack id", func() {
						it("should error", func() {
							subject.SetOrder(dist.Order{{
								Group: []dist.BuildpackRef{
									{BuildpackInfo: dist.BuildpackInfo{ID: "missing-buildpack-id"}}},
							}})

							err := subject.Save(logger)

							h.AssertError(t, err, "no versions of buildpack 'missing-buildpack-id' were found on the builder")
						})
					})

					when("order points to missing buildpack version", func() {
						it("should error", func() {
							subject.SetOrder(dist.Order{{
								Group: []dist.BuildpackRef{
									{BuildpackInfo: dist.BuildpackInfo{ID: "buildpack-1-id", Version: "missing-buildpack-version"}}},
							}})

							err := subject.Save(logger)

							h.AssertError(t, err, "buildpack 'buildpack-1-id' with version 'missing-buildpack-version' was not found on the builder")
						})
					})
				})

				when("has multiple buildpacks with same ID", func() {
					it.Before(func() {
						subject.AddBuildpack(bp1v1)
						subject.AddBuildpack(bp1v2)
					})

					when("order explicitly sets version", func() {
						it("should keep order version", func() {
							subject.SetOrder(dist.Order{{
								Group: []dist.BuildpackRef{
									{BuildpackInfo: bp1v1.Descriptor().Info}},
							}})

							err := subject.Save(logger)
							h.AssertNil(t, err)

							layerTar, err := baseImage.FindLayerWithPath("/cnb/order.toml")
							h.AssertNil(t, err)
							h.AssertOnTarEntry(t, layerTar, "/cnb/order.toml", h.ContentEquals(`[[order]]

  [[order.group]]
    id = "buildpack-1-id"
    version = "buildpack-1-version-1"
`))
						})
					})

					when("order version is empty", func() {
						it("return error", func() {
							subject.SetOrder(dist.Order{{
								Group: []dist.BuildpackRef{
									{BuildpackInfo: dist.BuildpackInfo{ID: "buildpack-1-id"}}},
							}})

							err := subject.Save(logger)
							h.AssertError(t, err, "multiple versions of 'buildpack-1-id' - must specify an explicit version")
						})
					})
				})
			})

			when("validating buildpacks", func() {
				when("nested buildpack does not exist", func() {
					when("buildpack by id does not exist", func() {
						it("returns an error", func() {
							subject.AddBuildpack(bp1v1)
							subject.AddBuildpack(bpOrder)

							// order buildpack requires bp2v1
							err := subject.Save(logger)

							h.AssertError(t, err, "buildpack 'buildpack-2-id@buildpack-2-version-1' not found on the builder")
						})
					})

					when("buildpack version does not exist", func() {
						it("returns an error", func() {
							subject.AddBuildpack(bp1v2)
							subject.AddBuildpack(bp2v1)

							// order buildpack requires bp1v1 rather than bp1v2
							subject.AddBuildpack(bpOrder)

							err := subject.Save(logger)

							h.AssertError(t, err, "buildpack 'buildpack-1-id@buildpack-1-version-1' not found on the builder")
						})
					})
				})

				when("buildpack stack id does not match", func() {
					it("returns an error", func() {
						subject.AddBuildpack(&fakeBuildpack{
							descriptor: dist.BuildpackDescriptor{
								API:    api.MustParse("0.2"),
								Info:   bp1v1.Descriptor().Info,
								Stacks: []dist.Stack{{ID: "other.stack.id"}},
							}})

						err := subject.Save(logger)

						h.AssertError(t, err, "buildpack 'buildpack-1-id@buildpack-1-version-1' does not support stack 'some.stack.id'")
					})
				})

				when("buildpack is not compatible with lifecycle", func() {
					it("returns an error", func() {
						subject.AddBuildpack(&fakeBuildpack{
							descriptor: dist.BuildpackDescriptor{
								API:    api.MustParse("0.1"),
								Info:   bp1v1.Descriptor().Info,
								Stacks: []dist.Stack{{ID: "some.stack.id"}},
							}})

						err := subject.Save(logger)

						h.AssertError(t, err, "buildpack 'buildpack-1-id@buildpack-1-version-1' (Buildpack API version 0.1) is incompatible with lifecycle '1.2.3' (Buildpack API version 0.2)")
					})
				})
			})
		})

		when("#SetLifecycle", func() {
			it.Before(func() {
				h.AssertNil(t, subject.SetLifecycle(mockLifecycle))

				h.AssertNil(t, subject.Save(logger))
				h.AssertEq(t, baseImage.IsSaved(), true)
			})

			it("should set the lifecycle version successfully", func() {
				h.AssertEq(t, subject.GetLifecycleDescriptor().Info.Version.String(), "1.2.3")
			})

			it("should add the lifecycle binaries as an image layer", func() {
				layerTar, err := baseImage.FindLayerWithPath("/cnb/lifecycle")
				h.AssertNil(t, err)
				h.AssertOnTarEntry(t, layerTar, "/cnb/lifecycle",
					h.IsDirectory(),
					h.HasFileMode(0755),
					h.HasModTime(archive.NormalizedDateTime),
				)

				h.AssertOnTarEntry(t, layerTar, "/cnb/lifecycle/detector",
					h.ContentEquals("detector"),
					h.HasFileMode(0755),
					h.HasModTime(archive.NormalizedDateTime),
				)

				h.AssertOnTarEntry(t, layerTar, "/cnb/lifecycle/restorer",
					h.ContentEquals("restorer"),
					h.HasFileMode(0755),
					h.HasModTime(archive.NormalizedDateTime),
				)

				h.AssertOnTarEntry(t, layerTar, "/cnb/lifecycle/analyzer",
					h.ContentEquals("analyzer"),
					h.HasFileMode(0755),
					h.HasModTime(archive.NormalizedDateTime),
				)

				h.AssertOnTarEntry(t, layerTar, "/cnb/lifecycle/builder",
					h.ContentEquals("builder"),
					h.HasFileMode(0755),
					h.HasModTime(archive.NormalizedDateTime),
				)

				h.AssertOnTarEntry(t, layerTar, "/cnb/lifecycle/exporter",
					h.ContentEquals("exporter"),
					h.HasFileMode(0755),
					h.HasModTime(archive.NormalizedDateTime),
				)

				h.AssertOnTarEntry(t, layerTar, "/cnb/lifecycle/cacher",
					h.ContentEquals("cacher"),
					h.HasFileMode(0755),
					h.HasModTime(archive.NormalizedDateTime),
				)

				h.AssertOnTarEntry(t, layerTar, "/cnb/lifecycle/launcher",
					h.ContentEquals("launcher"),
					h.HasFileMode(0755),
					h.HasModTime(archive.NormalizedDateTime),
				)
			})

			it("sets the lifecycle version on the metadata", func() {
				label, err := baseImage.Label("io.buildpacks.builder.metadata")
				h.AssertNil(t, err)

				var metadata builder.Metadata
				h.AssertNil(t, json.Unmarshal([]byte(label), &metadata))
				h.AssertEq(t, metadata.Lifecycle.Version.String(), "1.2.3")
				h.AssertEq(t, metadata.Lifecycle.API.PlatformVersion.String(), "2.2")
				h.AssertEq(t, metadata.Lifecycle.API.BuildpackVersion.String(), "0.2")
			})
		})

		when("#AddBuildpack", func() {
			it.Before(func() {
				subject.AddBuildpack(bp1v1)
				subject.AddBuildpack(bp1v2)
				subject.AddBuildpack(bp2v1)
				subject.AddBuildpack(bpOrder)
			})

			it("adds the buildpack as an image layer", func() {
				h.AssertNil(t, subject.Save(logger))
				h.AssertEq(t, baseImage.IsSaved(), true)
				assertImageHasBPLayer(t, baseImage, bp1v1)
				assertImageHasBPLayer(t, baseImage, bp1v2)
				assertImageHasBPLayer(t, baseImage, bp2v1)
				assertImageHasBPLayer(t, baseImage, bpOrder)
			})

			it("adds the buildpack metadata", func() {
				h.AssertNil(t, subject.Save(logger))
				h.AssertEq(t, baseImage.IsSaved(), true)

				label, err := baseImage.Label("io.buildpacks.builder.metadata")
				h.AssertNil(t, err)

				var metadata builder.Metadata
				h.AssertNil(t, json.Unmarshal([]byte(label), &metadata))
				h.AssertEq(t, len(metadata.Buildpacks), 4)

				h.AssertEq(t, metadata.Buildpacks[0].ID, "buildpack-1-id")
				h.AssertEq(t, metadata.Buildpacks[0].Version, "buildpack-1-version-1")
				h.AssertEq(t, metadata.Buildpacks[0].Latest, false)

				h.AssertEq(t, metadata.Buildpacks[1].ID, "buildpack-1-id")
				h.AssertEq(t, metadata.Buildpacks[1].Version, "buildpack-1-version-2")
				h.AssertEq(t, metadata.Buildpacks[1].Latest, false)

				h.AssertEq(t, metadata.Buildpacks[2].ID, "buildpack-2-id")
				h.AssertEq(t, metadata.Buildpacks[2].Version, "buildpack-2-version-1")
				h.AssertEq(t, metadata.Buildpacks[2].Latest, true)

				h.AssertEq(t, metadata.Buildpacks[3].ID, "order-buildpack-id")
				h.AssertEq(t, metadata.Buildpacks[3].Version, "order-buildpack-version")
				h.AssertEq(t, metadata.Buildpacks[3].Latest, true)
			})

			it("adds the buildpack layers label", func() {
				h.AssertNil(t, subject.Save(logger))
				h.AssertEq(t, baseImage.IsSaved(), true)

				label, err := baseImage.Label("io.buildpacks.buildpack.layers")
				h.AssertNil(t, err)

				var layers builder.BuildpackLayers
				h.AssertNil(t, json.Unmarshal([]byte(label), &layers))
				h.AssertEq(t, len(layers), 3)
				h.AssertEq(t, len(layers["buildpack-1-id"]), 2)
				h.AssertEq(t, len(layers["buildpack-2-id"]), 1)

				h.AssertUnique(t,
					layers["buildpack-1-id"]["buildpack-1-version-1"].LayerDigest,
					layers["buildpack-1-id"]["buildpack-1-version-2"].LayerDigest,
					layers["buildpack-2-id"]["buildpack-2-version-1"].LayerDigest,
					layers["order-buildpack-id"]["order-buildpack-version"].LayerDigest,
				)

				h.AssertEq(t, len(layers["buildpack-1-id"]["buildpack-1-version-1"].Order), 0)
				h.AssertEq(t, len(layers["buildpack-1-id"]["buildpack-1-version-2"].Order), 0)
				h.AssertEq(t, len(layers["buildpack-2-id"]["buildpack-2-version-1"].Order), 0)
				h.AssertEq(t, len(layers["order-buildpack-id"]["order-buildpack-version"].Order), 1)
				h.AssertEq(t, len(layers["order-buildpack-id"]["order-buildpack-version"].Order[0].Group), 2)
				h.AssertEq(t, layers["order-buildpack-id"]["order-buildpack-version"].Order[0].Group[0].ID, "buildpack-1-id")
				h.AssertEq(t, layers["order-buildpack-id"]["order-buildpack-version"].Order[0].Group[0].Version, "buildpack-1-version-1")
				h.AssertEq(t, layers["order-buildpack-id"]["order-buildpack-version"].Order[0].Group[0].Optional, true)
				h.AssertEq(t, layers["order-buildpack-id"]["order-buildpack-version"].Order[0].Group[1].ID, "buildpack-2-id")
				h.AssertEq(t, layers["order-buildpack-id"]["order-buildpack-version"].Order[0].Group[1].Version, "buildpack-2-version-1")
				h.AssertEq(t, layers["order-buildpack-id"]["order-buildpack-version"].Order[0].Group[1].Optional, false)
			})

			when("base image already has buildpack layers label", func() {
				it.Before(func() {
					h.AssertNil(t, baseImage.SetLabel(
						"io.buildpacks.buildpack.layers",
						`{ "buildpack-1-id": { "buildpack-1-version-1": { "layerDigest": "sha256:buildpack-1-version-1-sha" }, "buildpack-1-version-2": { "layerDigest": "sha256:buildpack-1-version-2-orig-sha" } } }`,
					))

					var err error
					subject, err = builder.New(baseImage, "some/builder")
					h.AssertNil(t, err)

					subject.AddBuildpack(bp1v2)
					subject.AddBuildpack(bp2v1)

					h.AssertNil(t, subject.SetLifecycle(mockLifecycle))

					h.AssertNil(t, subject.Save(logger))
					h.AssertEq(t, baseImage.IsSaved(), true)
				})

				it("appends buildpack layer info", func() {
					label, err := baseImage.Label("io.buildpacks.buildpack.layers")
					h.AssertNil(t, err)

					var layers builder.BuildpackLayers
					h.AssertNil(t, json.Unmarshal([]byte(label), &layers))
					h.AssertEq(t, len(layers), 2)
					h.AssertEq(t, len(layers["buildpack-1-id"]), 2)
					h.AssertEq(t, len(layers["buildpack-2-id"]), 1)

					h.AssertEq(t, layers["buildpack-1-id"]["buildpack-1-version-1"].LayerDigest, "sha256:buildpack-1-version-1-sha")
					h.AssertUnique(t,
						layers["buildpack-1-id"]["buildpack-1-version-1"].LayerDigest,
						layers["buildpack-1-id"]["buildpack-1-version-2"].LayerDigest,
						layers["buildpack-2-id"]["buildpack-2-version-1"].LayerDigest,
					)

					h.AssertMatch(t, layers["buildpack-1-id"]["buildpack-1-version-1"].LayerDigest, "^sha256:.*")
					h.AssertMatch(t, layers["buildpack-1-id"]["buildpack-1-version-2"].LayerDigest, "^sha256:.*")
					h.AssertMatch(t, layers["buildpack-2-id"]["buildpack-2-version-1"].LayerDigest, "^sha256:.*")

					h.AssertEq(t, len(layers["buildpack-1-id"]["buildpack-1-version-1"].Order), 0)
					h.AssertEq(t, len(layers["buildpack-1-id"]["buildpack-1-version-2"].Order), 0)
					h.AssertEq(t, len(layers["buildpack-2-id"]["buildpack-2-version-1"].Order), 0)
				})

				it("warns when overriding existing buildpack", func() {
					label, err := baseImage.Label("io.buildpacks.buildpack.layers")
					h.AssertNil(t, err)

					var layers builder.BuildpackLayers
					h.AssertNil(t, json.Unmarshal([]byte(label), &layers))

					h.AssertContains(t, buf.String(), "Warning: buildpack 'buildpack-1-id@buildpack-1-version-2' already exists on builder and will be overridden")
					h.AssertNotContains(t, layers["buildpack-1-id"]["buildpack-1-version-2"].LayerDigest, "buildpack-1-version-2-orig-sha")
				})
			})

			when("base image already has metadata", func() {
				it.Before(func() {
					h.AssertNil(t, baseImage.SetLabel(
						"io.buildpacks.builder.metadata",
						`{"buildpacks": [{"id": "prev.id"}], "groups": [{"buildpacks": [{"id": "prev.id"}]}], "stack": {"runImage": {"image": "prev/run", "mirrors": ["prev/mirror"]}}, "lifecycle": {"version": "6.6.6", "api": {"buildpack": "0.2", "platform": "2.2"}}}`,
					))

					var err error
					subject, err = builder.New(baseImage, "some/builder")
					h.AssertNil(t, err)

					subject.AddBuildpack(bp1v1)
					h.AssertNil(t, subject.Save(logger))
					h.AssertEq(t, baseImage.IsSaved(), true)
				})

				it("appends the buildpack to the metadata", func() {
					label, err := baseImage.Label("io.buildpacks.builder.metadata")
					h.AssertNil(t, err)

					var metadata builder.Metadata
					h.AssertNil(t, json.Unmarshal([]byte(label), &metadata))
					h.AssertEq(t, len(metadata.Buildpacks), 2)

					// keeps original metadata
					h.AssertEq(t, metadata.Buildpacks[0].ID, "prev.id")
					h.AssertEq(t, metadata.Stack.RunImage.Image, "prev/run")
					h.AssertEq(t, metadata.Stack.RunImage.Mirrors[0], "prev/mirror")
					h.AssertEq(t, subject.GetLifecycleDescriptor().Info.Version.String(), "6.6.6")

					// adds new buildpack
					h.AssertEq(t, metadata.Buildpacks[1].ID, "buildpack-1-id")
					h.AssertEq(t, metadata.Buildpacks[1].Version, "buildpack-1-version-1")
					h.AssertEq(t, metadata.Buildpacks[1].Latest, true)
				})
			})
		})

		when("#SetOrder", func() {
			when("the buildpacks exist in the image", func() {
				it.Before(func() {
					subject.AddBuildpack(bp1v1)
					subject.AddBuildpack(bp2v1)
					subject.SetOrder(dist.Order{
						{Group: []dist.BuildpackRef{
							{
								BuildpackInfo: dist.BuildpackInfo{
									ID: bp1v1.Descriptor().Info.ID,
									// Version excluded intentionally
								},
							},
							{
								BuildpackInfo: bp2v1.Descriptor().Info,
								Optional:      true,
							},
						}},
					})

					h.AssertNil(t, subject.Save(logger))
					h.AssertEq(t, baseImage.IsSaved(), true)
				})

				it("adds the order.toml to the image", func() {
					layerTar, err := baseImage.FindLayerWithPath("/cnb/order.toml")
					h.AssertNil(t, err)
					h.AssertOnTarEntry(t, layerTar, "/cnb/order.toml",
						h.ContentEquals(`[[order]]

  [[order.group]]
    id = "buildpack-1-id"
    version = "buildpack-1-version-1"

  [[order.group]]
    id = "buildpack-2-id"
    version = "buildpack-2-version-1"
    optional = true
`),
						h.HasModTime(archive.NormalizedDateTime),
					)
				})

				it("adds the order to the order label", func() {
					label, err := baseImage.Label("io.buildpacks.buildpack.order")
					h.AssertNil(t, err)

					var order dist.Order
					h.AssertNil(t, json.Unmarshal([]byte(label), &order))
					h.AssertEq(t, len(order), 1)
					h.AssertEq(t, len(order[0].Group), 2)
					h.AssertEq(t, order[0].Group[0].ID, "buildpack-1-id")
					h.AssertEq(t, order[0].Group[0].Version, "")
					h.AssertEq(t, order[0].Group[0].Optional, false)
					h.AssertEq(t, order[0].Group[1].ID, "buildpack-2-id")
					h.AssertEq(t, order[0].Group[1].Version, "buildpack-2-version-1")
					h.AssertEq(t, order[0].Group[1].Optional, true)
				})
			})
		})

		when("#SetDescription", func() {
			it.Before(func() {
				subject.SetDescription("Some description")
				h.AssertNil(t, subject.Save(logger))
				h.AssertEq(t, baseImage.IsSaved(), true)
			})

			it("sets the description on the metadata", func() {
				label, err := baseImage.Label("io.buildpacks.builder.metadata")
				h.AssertNil(t, err)

				var metadata builder.Metadata
				h.AssertNil(t, json.Unmarshal([]byte(label), &metadata))
				h.AssertEq(t, metadata.Description, "Some description")
			})
		})

		when("#SetStackInfo", func() {
			it.Before(func() {
				subject.SetStackInfo(builder.StackConfig{
					RunImage:        "some/run",
					RunImageMirrors: []string{"some/mirror", "other/mirror"},
				})
				h.AssertNil(t, subject.Save(logger))
				h.AssertEq(t, baseImage.IsSaved(), true)
			})

			it("adds the stack.toml to the image", func() {
				layerTar, err := baseImage.FindLayerWithPath("/cnb/stack.toml")
				h.AssertNil(t, err)
				h.AssertOnTarEntry(t, layerTar, "/cnb/stack.toml",
					h.ContentEquals(`[run-image]
  image = "some/run"
  mirrors = ["some/mirror", "other/mirror"]
`),
					h.HasModTime(archive.NormalizedDateTime),
				)
			})

			it("adds the stack to the metadata", func() {
				label, err := baseImage.Label("io.buildpacks.builder.metadata")
				h.AssertNil(t, err)

				var metadata builder.Metadata
				h.AssertNil(t, json.Unmarshal([]byte(label), &metadata))
				h.AssertEq(t, metadata.Stack.RunImage.Image, "some/run")
				h.AssertEq(t, metadata.Stack.RunImage.Mirrors[0], "some/mirror")
				h.AssertEq(t, metadata.Stack.RunImage.Mirrors[1], "other/mirror")
			})
		})

		when("#SetEnv", func() {
			it.Before(func() {
				subject.SetEnv(map[string]string{
					"SOME_KEY":  "some-val",
					"OTHER_KEY": "other-val",
				})
				h.AssertNil(t, subject.Save(logger))
				h.AssertEq(t, baseImage.IsSaved(), true)
			})

			it("adds the env vars as files to the image", func() {
				layerTar, err := baseImage.FindLayerWithPath("/platform/env/SOME_KEY")
				h.AssertNil(t, err)
				h.AssertOnTarEntry(t, layerTar, "/platform/env/SOME_KEY",
					h.ContentEquals(`some-val`),
					h.HasModTime(archive.NormalizedDateTime),
				)
				h.AssertOnTarEntry(t, layerTar, "/platform/env/OTHER_KEY",
					h.ContentEquals(`other-val`),
					h.HasModTime(archive.NormalizedDateTime),
				)
			})
		})
	})

	when("builder exists", func() {
		var builderImage imgutil.Image

		it.Before(func() {
			h.AssertNil(t, baseImage.SetEnv("CNB_USER_ID", "1234"))
			h.AssertNil(t, baseImage.SetEnv("CNB_GROUP_ID", "4321"))
			h.AssertNil(t, baseImage.SetLabel("io.buildpacks.stack.id", "some.stack.id"))
			h.AssertNil(t, baseImage.SetLabel(
				"io.buildpacks.builder.metadata",
				`{"buildpacks": [{"id": "buildpack-1-id"}, {"id": "buildpack-2-id"}], "groups": [{"buildpacks": [{"id": "buildpack-1-id", "version": "buildpack-1-version", "optional": false}, {"id": "buildpack-2-id", "version": "buildpack-2-version-1", "optional": true}]}], "stack": {"runImage": {"image": "prev/run", "mirrors": ["prev/mirror"]}}, "lifecycle": {"version": "6.6.6"}}`,
			))
			h.AssertNil(t, baseImage.SetLabel(
				"io.buildpacks.buildpack.order",
				`[{"group": [{"id": "buildpack-1-id", "optional": false}, {"id": "buildpack-2-id", "version": "buildpack-2-version-1", "optional": true}]}]`,
			))

			builderImage = baseImage
		})

		when("#GetBuilder", func() {
			it("gets builder from image", func() {
				bldr, err := builder.GetBuilder(builderImage)
				h.AssertNil(t, err)
				h.AssertEq(t, bldr.GetBuildpacks()[0].ID, "buildpack-1-id")
				h.AssertEq(t, bldr.GetBuildpacks()[1].ID, "buildpack-2-id")

				order := bldr.GetOrder()
				h.AssertEq(t, len(order), 1)
				h.AssertEq(t, len(order[0].Group), 2)
				h.AssertEq(t, order[0].Group[0].ID, "buildpack-1-id")
				h.AssertEq(t, order[0].Group[0].Version, "")
				h.AssertEq(t, order[0].Group[0].Optional, false)
				h.AssertEq(t, order[0].Group[1].ID, "buildpack-2-id")
				h.AssertEq(t, order[0].Group[1].Version, "buildpack-2-version-1")
				h.AssertEq(t, order[0].Group[1].Optional, true)
			})

			when("metadata is missing", func() {
				it.Before(func() {
					h.AssertNil(t, builderImage.SetLabel(
						"io.buildpacks.builder.metadata",
						"",
					))
				})

				it("should error", func() {
					_, err := builder.GetBuilder(builderImage)
					h.AssertError(t, err, "missing label 'io.buildpacks.builder.metadata'")
				})
			})
		})
	})
}

type fakeBuildpack struct {
	descriptor dist.BuildpackDescriptor
}

func (f *fakeBuildpack) Descriptor() dist.BuildpackDescriptor {
	return f.descriptor
}

func (f *fakeBuildpack) Open() (io.ReadCloser, error) {
	return archive.ReadDirAsTar(filepath.Join("testdata", "buildpack"), ".", 0, 0, 0755), nil
}

func assertImageHasBPLayer(t *testing.T, image *fakes.Image, bp dist.Buildpack) {
	dirPath := fmt.Sprintf("/cnb/buildpacks/%s/%s", bp.Descriptor().Info.ID, bp.Descriptor().Info.Version)
	layerTar, err := image.FindLayerWithPath(dirPath)
	h.AssertNil(t, err)

	h.AssertOnTarEntry(t, layerTar, dirPath,
		h.IsDirectory(),
		h.HasModTime(archive.NormalizedDateTime),
	)

	h.AssertOnTarEntry(t, layerTar, path.Dir(dirPath),
		h.IsDirectory(),
		h.HasModTime(archive.NormalizedDateTime),
	)

	h.AssertOnTarEntry(t, layerTar, dirPath+"/bin/build",
		h.ContentEquals("build-contents"),
		h.HasOwnerAndGroup(1234, 4321),
		h.HasFileMode(0755),
	)

	h.AssertOnTarEntry(t, layerTar, dirPath+"/bin/detect",
		h.ContentEquals("detect-contents"),
		h.HasOwnerAndGroup(1234, 4321),
		h.HasFileMode(0755),
	)
}
