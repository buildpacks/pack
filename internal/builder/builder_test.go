package builder_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path"
	"path/filepath"
	"testing"

	"github.com/buildpacks/imgutil"
	"github.com/buildpacks/imgutil/fakes"
	"github.com/buildpacks/lifecycle/api"
	"github.com/golang/mock/gomock"
	"github.com/heroku/color"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	pubbldr "github.com/buildpacks/pack/builder"
	"github.com/buildpacks/pack/internal/archive"
	"github.com/buildpacks/pack/internal/builder"
	"github.com/buildpacks/pack/internal/builder/testmocks"
	"github.com/buildpacks/pack/internal/dist"
	ifakes "github.com/buildpacks/pack/internal/fakes"
	ilogging "github.com/buildpacks/pack/internal/logging"
	"github.com/buildpacks/pack/logging"
	h "github.com/buildpacks/pack/testhelpers"
)

func TestBuilder(t *testing.T) {
	color.Disable(true)
	defer color.Disable(false)
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
		outBuf         bytes.Buffer
		logger         logging.Logger
	)

	it.Before(func() {
		logger = ilogging.NewLogWithWriters(&outBuf, &outBuf)
		baseImage = fakes.NewImage("base/image", "", nil)
		mockController = gomock.NewController(t)

		lifecycleTarReader := archive.ReadDirAsTar(
			filepath.Join("testdata", "lifecycle", "platform-0.4"),
			".", 0, 0, 0755, true, nil,
		)

		descriptorContents, err := ioutil.ReadFile(filepath.Join("testdata", "lifecycle", "platform-0.4", "lifecycle.toml"))
		h.AssertNil(t, err)

		lifecycleDescriptor, err := builder.ParseDescriptor(string(descriptorContents))
		h.AssertNil(t, err)

		mockLifecycle = testmocks.NewMockLifecycle(mockController)
		mockLifecycle.EXPECT().Open().Return(lifecycleTarReader, nil).AnyTimes()
		mockLifecycle.EXPECT().Descriptor().Return(builder.CompatDescriptor(lifecycleDescriptor)).AnyTimes()

		bp1v1, err = ifakes.NewFakeBuildpack(dist.BuildpackDescriptor{
			API: api.MustParse("0.2"),
			Info: dist.BuildpackInfo{
				ID:      "buildpack-1-id",
				Version: "buildpack-1-version-1",
			},
			Stacks: []dist.Stack{{
				ID:     "some.stack.id",
				Mixins: []string{"mixinX", "mixinY"},
			}},
		}, 0644)
		h.AssertNil(t, err)

		bp1v2, err = ifakes.NewFakeBuildpack(dist.BuildpackDescriptor{
			API: api.MustParse("0.2"),
			Info: dist.BuildpackInfo{
				ID:      "buildpack-1-id",
				Version: "buildpack-1-version-2",
			},
			Stacks: []dist.Stack{{
				ID:     "some.stack.id",
				Mixins: []string{"mixinX", "mixinY"},
			}},
		}, 0644)
		h.AssertNil(t, err)

		bp2v1, err = ifakes.NewFakeBuildpack(dist.BuildpackDescriptor{
			API: api.MustParse("0.2"),
			Info: dist.BuildpackInfo{
				ID:      "buildpack-2-id",
				Version: "buildpack-2-version-1",
			},
			Stacks: []dist.Stack{{
				ID:     "some.stack.id",
				Mixins: []string{"build:mixinA", "run:mixinB"},
			}},
		}, 0644)
		h.AssertNil(t, err)

		bpOrder, err = ifakes.NewFakeBuildpack(dist.BuildpackDescriptor{
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
		}, 0644)
		h.AssertNil(t, err)
	})

	it.After(func() {
		baseImage.Cleanup()
		mockController.Finish()
	})

	when("the base image is not valid", func() {
		when("#FromImage", func() {
			when("metadata isn't valid", func() {
				it("returns an error", func() {
					h.AssertNil(t, baseImage.SetLabel(
						"io.buildpacks.builder.metadata",
						`{"something-random": ,}`,
					))

					_, err := builder.FromImage(baseImage)
					h.AssertError(t, err, "getting label")
				})
			})
		})

		when("#New", func() {
			when("metadata isn't valid", func() {
				it("returns an error", func() {
					h.AssertNil(t, baseImage.SetLabel(
						"io.buildpacks.builder.metadata",
						`{"something-random": ,}`,
					))

					_, err := builder.FromImage(baseImage)
					h.AssertError(t, err, "getting label")
				})
			})

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

			when("mixins metadata is malformed", func() {
				it.Before(func() {
					h.AssertNil(t, baseImage.SetEnv("CNB_USER_ID", "1234"))
					h.AssertNil(t, baseImage.SetEnv("CNB_GROUP_ID", "4321"))
					h.AssertNil(t, baseImage.SetLabel("io.buildpacks.stack.id", "some-id"))
				})

				it("returns an error", func() {
					h.AssertNil(t, baseImage.SetLabel("io.buildpacks.stack.mixins", `{"mixinX", "mixinY", "build:mixinA"}`))
					_, err := builder.New(baseImage, "some/builder")
					h.AssertError(t, err, "getting label io.buildpacks.stack.mixins")
				})
			})

			when("order metadata is malformed", func() {
				it.Before(func() {
					h.AssertNil(t, baseImage.SetEnv("CNB_USER_ID", "1234"))
					h.AssertNil(t, baseImage.SetEnv("CNB_GROUP_ID", "4321"))
					h.AssertNil(t, baseImage.SetLabel("io.buildpacks.stack.id", "some-id"))
				})

				it("returns an error", func() {
					h.AssertNil(t, baseImage.SetLabel("io.buildpacks.buildpack.order", `{"something", }`))
					_, err := builder.New(baseImage, "some/builder")
					h.AssertError(t, err, "getting label io.buildpacks.buildpack.order")
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
			h.AssertNil(t, baseImage.SetLabel("io.buildpacks.stack.mixins", `["mixinX", "mixinY", "build:mixinA"]`))
			subject, err = builder.New(baseImage, "some/builder")
			h.AssertNil(t, err)

			subject.SetLifecycle(mockLifecycle)
		})

		it.After(func() {
			baseImage.Cleanup()
		})

		when("#Save", func() {
			it("creates a builder from the image and renames it", func() {
				h.AssertNil(t, subject.Save(logger, builder.CreatorMetadata{}))
				h.AssertEq(t, baseImage.IsSaved(), true)
				h.AssertEq(t, baseImage.Name(), "some/builder")
			})

			it("adds creator metadata", func() {
				testName := "test-name"
				testVersion := "1.2.5"
				h.AssertNil(t, subject.Save(logger, builder.CreatorMetadata{
					Name:    testName,
					Version: testVersion,
				}))
				h.AssertEq(t, baseImage.IsSaved(), true)

				label, err := baseImage.Label("io.buildpacks.builder.metadata")
				h.AssertNil(t, err)

				var metadata builder.Metadata
				h.AssertNil(t, json.Unmarshal([]byte(label), &metadata))

				h.AssertEq(t, metadata.CreatedBy.Name, testName)
				h.AssertEq(t, metadata.CreatedBy.Version, testVersion)
			})

			it("adds creator name if not provided", func() {
				testVersion := "1.2.5"
				h.AssertNil(t, subject.Save(logger, builder.CreatorMetadata{
					Version: testVersion,
				}))
				h.AssertEq(t, baseImage.IsSaved(), true)

				label, err := baseImage.Label("io.buildpacks.builder.metadata")
				h.AssertNil(t, err)

				var metadata builder.Metadata
				h.AssertNil(t, json.Unmarshal([]byte(label), &metadata))

				h.AssertEq(t, metadata.CreatedBy.Name, "Pack CLI")
				h.AssertEq(t, metadata.CreatedBy.Version, testVersion)
			})

			it("creates the workspace dir with CNB user and group", func() {
				h.AssertNil(t, subject.Save(logger, builder.CreatorMetadata{}))
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
				h.AssertNil(t, subject.Save(logger, builder.CreatorMetadata{}))
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
				h.AssertNil(t, subject.Save(logger, builder.CreatorMetadata{}))
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
				h.AssertNil(t, subject.Save(logger, builder.CreatorMetadata{}))
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
				h.AssertNil(t, subject.Save(logger, builder.CreatorMetadata{}))
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
				h.AssertNil(t, subject.Save(logger, builder.CreatorMetadata{}))
				h.AssertEq(t, baseImage.IsSaved(), true)

				h.AssertEq(t, baseImage.WorkingDir(), "/layers")
			})

			it("does not overwrite the order layer when SetOrder has not been called", func() {
				tmpDir, err := ioutil.TempDir("", "")
				h.AssertNil(t, err)
				defer os.RemoveAll(tmpDir)

				layerFile := filepath.Join(tmpDir, "order.tar")
				err = archive.CreateSingleFileTar(layerFile, "/cnb/order.toml", "some content")
				h.AssertNil(t, err)

				h.AssertNil(t, baseImage.AddLayer(layerFile))
				baseImage.Save()

				h.AssertNil(t, subject.Save(logger, builder.CreatorMetadata{}))
				h.AssertEq(t, baseImage.IsSaved(), true)

				layerTar, err := baseImage.FindLayerWithPath("/cnb/order.toml")
				h.AssertNil(t, err)
				h.AssertOnTarEntry(t, layerTar, "/cnb/order.toml", h.ContentEquals("some content"))
			})

			when("validating order", func() {
				it.Before(func() {
					subject.SetLifecycle(mockLifecycle)
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

						err := subject.Save(logger, builder.CreatorMetadata{})
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

							err := subject.Save(logger, builder.CreatorMetadata{})

							h.AssertError(t, err, "no versions of buildpack 'missing-buildpack-id' were found on the builder")
						})
					})

					when("order points to missing buildpack version", func() {
						it("should error", func() {
							subject.SetOrder(dist.Order{{
								Group: []dist.BuildpackRef{
									{BuildpackInfo: dist.BuildpackInfo{ID: "buildpack-1-id", Version: "missing-buildpack-version"}}},
							}})

							err := subject.Save(logger, builder.CreatorMetadata{})

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

							err := subject.Save(logger, builder.CreatorMetadata{})
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

							err := subject.Save(logger, builder.CreatorMetadata{})
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
							err := subject.Save(logger, builder.CreatorMetadata{})

							h.AssertError(t, err, "buildpack 'buildpack-2-id@buildpack-2-version-1' not found on the builder")
						})
					})

					when("buildpack version does not exist", func() {
						it("returns an error", func() {
							subject.AddBuildpack(bp1v2)
							subject.AddBuildpack(bp2v1)

							// order buildpack requires bp1v1 rather than bp1v2
							subject.AddBuildpack(bpOrder)

							err := subject.Save(logger, builder.CreatorMetadata{})

							h.AssertError(t, err, "buildpack 'buildpack-1-id@buildpack-1-version-1' not found on the builder")
						})
					})
				})

				when("buildpack stack id does not match", func() {
					it("returns an error", func() {
						bp, err := ifakes.NewFakeBuildpack(dist.BuildpackDescriptor{
							API:    api.MustParse("0.2"),
							Info:   bp1v1.Descriptor().Info,
							Stacks: []dist.Stack{{ID: "other.stack.id"}},
						}, 0644)
						h.AssertNil(t, err)

						subject.AddBuildpack(bp)
						err = subject.Save(logger, builder.CreatorMetadata{})

						h.AssertError(t, err, "buildpack 'buildpack-1-id@buildpack-1-version-1' does not support stack 'some.stack.id'")
					})
				})

				when("buildpack is not compatible with lifecycle", func() {
					it("returns an error", func() {
						bp, err := ifakes.NewFakeBuildpack(dist.BuildpackDescriptor{
							API:    api.MustParse("0.1"),
							Info:   bp1v1.Descriptor().Info,
							Stacks: []dist.Stack{{ID: "some.stack.id"}},
						}, 0644)
						h.AssertNil(t, err)

						subject.AddBuildpack(bp)
						err = subject.Save(logger, builder.CreatorMetadata{})

						h.AssertError(t,
							err,
							"buildpack 'buildpack-1-id@buildpack-1-version-1' (Buildpack API 0.1) is incompatible with lifecycle '0.0.0' (Buildpack API(s) 0.2, 0.3, 0.4)")
					})
				})

				when("buildpack mixins are not satisfied", func() {
					it("returns an error", func() {
						bp, err := ifakes.NewFakeBuildpack(dist.BuildpackDescriptor{
							API:  api.MustParse("0.2"),
							Info: bp1v1.Descriptor().Info,
							Stacks: []dist.Stack{{
								ID:     "some.stack.id",
								Mixins: []string{"missing"},
							}},
						}, 0644)
						h.AssertNil(t, err)

						subject.AddBuildpack(bp)
						err = subject.Save(logger, builder.CreatorMetadata{})

						h.AssertError(t, err, "buildpack 'buildpack-1-id@buildpack-1-version-1' requires missing mixin(s): missing")
					})
				})
			})

			when("getting layers label", func() {
				it("fails if layers label isn't set correctly", func() {
					h.AssertNil(t, baseImage.SetLabel(
						"io.buildpacks.buildpack.layers",
						`{"something-here: }`,
					))

					err := subject.Save(logger, builder.CreatorMetadata{})
					h.AssertError(t, err, "getting label io.buildpacks.buildpack.layers")
				})
			})
		})

		when("#SetLifecycle", func() {
			it.Before(func() {
				h.AssertNil(t, subject.Save(logger, builder.CreatorMetadata{}))
				h.AssertEq(t, baseImage.IsSaved(), true)
			})

			it("should set the lifecycle version successfully", func() {
				h.AssertEq(t, subject.LifecycleDescriptor().Info.Version.String(), "0.0.0")
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
				h.AssertEq(t, metadata.Lifecycle.Version.String(), "0.0.0")
				h.AssertEq(t, metadata.Lifecycle.API.BuildpackVersion.String(), "0.2")
				h.AssertEq(t, metadata.Lifecycle.API.PlatformVersion.String(), "0.2")
				h.AssertNotNil(t, metadata.Lifecycle.APIs)
				h.AssertEq(t, metadata.Lifecycle.APIs.Buildpack.Deprecated.AsStrings(), []string{})
				h.AssertEq(t, metadata.Lifecycle.APIs.Buildpack.Supported.AsStrings(), []string{"0.2", "0.3", "0.4"})
				h.AssertEq(t, metadata.Lifecycle.APIs.Platform.Deprecated.AsStrings(), []string{"0.2"})
				h.AssertEq(t, metadata.Lifecycle.APIs.Platform.Supported.AsStrings(), []string{"0.3", "0.4"})
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
				h.AssertNil(t, subject.Save(logger, builder.CreatorMetadata{}))
				h.AssertEq(t, baseImage.IsSaved(), true)
				assertImageHasBPLayer(t, baseImage, bp1v1)
				assertImageHasBPLayer(t, baseImage, bp1v2)
				assertImageHasBPLayer(t, baseImage, bp2v1)
				assertImageHasOrderBpLayer(t, baseImage, bpOrder)
			})

			it("adds the buildpack metadata", func() {
				h.AssertNil(t, subject.Save(logger, builder.CreatorMetadata{}))
				h.AssertEq(t, baseImage.IsSaved(), true)

				label, err := baseImage.Label("io.buildpacks.builder.metadata")
				h.AssertNil(t, err)

				var metadata builder.Metadata
				h.AssertNil(t, json.Unmarshal([]byte(label), &metadata))
				h.AssertEq(t, len(metadata.Buildpacks), 4)

				h.AssertEq(t, metadata.Buildpacks[0].ID, "buildpack-1-id")
				h.AssertEq(t, metadata.Buildpacks[0].Version, "buildpack-1-version-1")

				h.AssertEq(t, metadata.Buildpacks[1].ID, "buildpack-1-id")
				h.AssertEq(t, metadata.Buildpacks[1].Version, "buildpack-1-version-2")

				h.AssertEq(t, metadata.Buildpacks[2].ID, "buildpack-2-id")
				h.AssertEq(t, metadata.Buildpacks[2].Version, "buildpack-2-version-1")

				h.AssertEq(t, metadata.Buildpacks[3].ID, "order-buildpack-id")
				h.AssertEq(t, metadata.Buildpacks[3].Version, "order-buildpack-version")
			})

			it("adds the buildpack layers label", func() {
				h.AssertNil(t, subject.Save(logger, builder.CreatorMetadata{}))
				h.AssertEq(t, baseImage.IsSaved(), true)

				label, err := baseImage.Label("io.buildpacks.buildpack.layers")
				h.AssertNil(t, err)

				var layers dist.BuildpackLayers
				h.AssertNil(t, json.Unmarshal([]byte(label), &layers))
				h.AssertEq(t, len(layers), 3)
				h.AssertEq(t, len(layers["buildpack-1-id"]), 2)
				h.AssertEq(t, len(layers["buildpack-2-id"]), 1)

				h.AssertEq(t, len(layers["buildpack-1-id"]["buildpack-1-version-1"].Order), 0)
				h.AssertEq(t, len(layers["buildpack-1-id"]["buildpack-1-version-1"].Stacks), 1)
				h.AssertEq(t, layers["buildpack-1-id"]["buildpack-1-version-1"].Stacks[0].ID, "some.stack.id")
				h.AssertSliceContainsOnly(t, layers["buildpack-1-id"]["buildpack-1-version-1"].Stacks[0].Mixins, "mixinX", "mixinY")

				h.AssertEq(t, len(layers["buildpack-1-id"]["buildpack-1-version-2"].Order), 0)
				h.AssertEq(t, len(layers["buildpack-1-id"]["buildpack-1-version-2"].Stacks), 1)
				h.AssertEq(t, layers["buildpack-1-id"]["buildpack-1-version-2"].Stacks[0].ID, "some.stack.id")
				h.AssertSliceContainsOnly(t, layers["buildpack-1-id"]["buildpack-1-version-2"].Stacks[0].Mixins, "mixinX", "mixinY")

				h.AssertEq(t, len(layers["buildpack-2-id"]["buildpack-2-version-1"].Order), 0)
				h.AssertEq(t, len(layers["buildpack-2-id"]["buildpack-2-version-1"].Stacks), 1)
				h.AssertEq(t, layers["buildpack-2-id"]["buildpack-2-version-1"].Stacks[0].ID, "some.stack.id")
				h.AssertSliceContainsOnly(t, layers["buildpack-2-id"]["buildpack-2-version-1"].Stacks[0].Mixins, "build:mixinA", "run:mixinB")

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
					var mdJSON bytes.Buffer
					h.AssertNil(t, json.Compact(
						&mdJSON,
						[]byte(`{
  "buildpack-1-id": {
    "buildpack-1-version-1": {
      "layerDiffID": "sha256:buildpack-1-version-1-diff-id"
    },
    "buildpack-1-version-2": {
      "layerDiffID": "sha256:buildpack-1-version-2-diff-id"
    }
  }
}
`)))

					h.AssertNil(t, baseImage.SetLabel(
						"io.buildpacks.buildpack.layers",
						mdJSON.String(),
					))

					var err error
					subject, err = builder.New(baseImage, "some/builder")
					h.AssertNil(t, err)

					subject.AddBuildpack(bp1v2)
					subject.AddBuildpack(bp2v1)

					subject.SetLifecycle(mockLifecycle)
				})

				it("appends buildpack layer info", func() {
					h.AssertNil(t, subject.Save(logger, builder.CreatorMetadata{}))
					h.AssertEq(t, baseImage.IsSaved(), true)

					label, err := baseImage.Label("io.buildpacks.buildpack.layers")
					h.AssertNil(t, err)

					var layers dist.BuildpackLayers
					h.AssertNil(t, json.Unmarshal([]byte(label), &layers))
					h.AssertEq(t, len(layers), 2)
					h.AssertEq(t, len(layers["buildpack-1-id"]), 2)
					h.AssertEq(t, len(layers["buildpack-2-id"]), 1)

					h.AssertEq(t, layers["buildpack-1-id"]["buildpack-1-version-1"].LayerDiffID, "sha256:buildpack-1-version-1-diff-id")

					h.AssertUnique(t,
						layers["buildpack-1-id"]["buildpack-1-version-1"].LayerDiffID,
						layers["buildpack-1-id"]["buildpack-1-version-2"].LayerDiffID,
						layers["buildpack-2-id"]["buildpack-2-version-1"].LayerDiffID,
					)

					h.AssertEq(t, len(layers["buildpack-1-id"]["buildpack-1-version-1"].Order), 0)
					h.AssertEq(t, len(layers["buildpack-1-id"]["buildpack-1-version-2"].Order), 0)
					h.AssertEq(t, len(layers["buildpack-2-id"]["buildpack-2-version-1"].Order), 0)
				})

				it("informs when overriding existing buildpack, and log level is DEBUG", func() {
					logger := ilogging.NewLogWithWriters(&outBuf, &outBuf, ilogging.WithVerbose())

					h.AssertNil(t, subject.Save(logger, builder.CreatorMetadata{}))
					h.AssertEq(t, baseImage.IsSaved(), true)

					label, err := baseImage.Label("io.buildpacks.buildpack.layers")
					h.AssertNil(t, err)

					var layers dist.BuildpackLayers
					h.AssertNil(t, json.Unmarshal([]byte(label), &layers))

					h.AssertContains(t,
						outBuf.String(),
						"buildpack 'buildpack-1-id@buildpack-1-version-2' already exists on builder and will be overwritten",
					)
					h.AssertNotContains(t, layers["buildpack-1-id"]["buildpack-1-version-2"].LayerDiffID, "buildpack-1-version-2-diff-id")
				})

				it("doesn't message when overriding existing buildpack when log level is INFO", func() {
					h.AssertNil(t, subject.Save(logger, builder.CreatorMetadata{}))
					h.AssertEq(t, baseImage.IsSaved(), true)

					label, err := baseImage.Label("io.buildpacks.buildpack.layers")
					h.AssertNil(t, err)

					var layers dist.BuildpackLayers
					h.AssertNil(t, json.Unmarshal([]byte(label), &layers))

					h.AssertNotContains(t,
						outBuf.String(),
						"buildpack 'buildpack-1-id@buildpack-1-version-2' already exists on builder and will be overwritten",
					)
					h.AssertNotContains(t, layers["buildpack-1-id"]["buildpack-1-version-2"].LayerDiffID, "buildpack-1-version-2-diff-id")
				})
			})

			when("base image already has metadata", func() {
				it.Before(func() {
					h.AssertNil(t, baseImage.SetLabel(
						"io.buildpacks.builder.metadata",
						`{
"buildpacks":[{"id":"prev.id"}],
"groups":[{"buildpacks":[{"id":"prev.id"}]}],
"stack":{"runImage":{"image":"prev/run","mirrors":["prev/mirror"]}},
"lifecycle":{"version":"6.6.6","apis":{"buildpack":{"deprecated":["0.1"],"supported":["0.2","0.3"]},"platform":{"deprecated":[],"supported":["2.3","2.4"]}}}
}`,
					))

					var err error
					subject, err = builder.New(baseImage, "some/builder")
					h.AssertNil(t, err)

					subject.AddBuildpack(bp1v1)
					h.AssertNil(t, subject.Save(logger, builder.CreatorMetadata{}))
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
					h.AssertEq(t, subject.LifecycleDescriptor().Info.Version.String(), "6.6.6")

					// adds new buildpack
					h.AssertEq(t, metadata.Buildpacks[1].ID, "buildpack-1-id")
					h.AssertEq(t, metadata.Buildpacks[1].Version, "buildpack-1-version-1")
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

					h.AssertNil(t, subject.Save(logger, builder.CreatorMetadata{}))
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
				h.AssertNil(t, subject.Save(logger, builder.CreatorMetadata{}))
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

		when("#SetStack", func() {
			it.Before(func() {
				subject.SetStack(pubbldr.StackConfig{
					RunImage:        "some/run",
					RunImageMirrors: []string{"some/mirror", "other/mirror"},
				})
				h.AssertNil(t, subject.Save(logger, builder.CreatorMetadata{}))
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
				h.AssertNil(t, subject.Save(logger, builder.CreatorMetadata{}))
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
			h.AssertNil(t, baseImage.SetLabel("io.buildpacks.stack.mixins", `["mixinX", "mixinY", "build:mixinA"]`))
			h.AssertNil(t, baseImage.SetLabel(
				"io.buildpacks.builder.metadata",
				`{"description": "some-description", "createdBy": {"name": "some-name", "version": "1.2.3"}, "buildpacks": [{"id": "buildpack-1-id"}, {"id": "buildpack-2-id"}], "groups": [{"buildpacks": [{"id": "buildpack-1-id", "version": "buildpack-1-version", "optional": false}, {"id": "buildpack-2-id", "version": "buildpack-2-version-1", "optional": true}]}], "stack": {"runImage": {"image": "prev/run", "mirrors": ["prev/mirror"]}}, "lifecycle": {"version": "6.6.6"}}`,
			))
			h.AssertNil(t, baseImage.SetLabel(
				"io.buildpacks.buildpack.order",
				`[{"group": [{"id": "buildpack-1-id", "optional": false}, {"id": "buildpack-2-id", "version": "buildpack-2-version-1", "optional": true}]}]`,
			))

			builderImage = baseImage
		})

		when("#FromImage", func() {
			var bldr *builder.Builder

			it.Before(func() {
				var err error
				bldr, err = builder.FromImage(builderImage)
				h.AssertNil(t, err)
			})

			it("gets builder from image", func() {
				h.AssertEq(t, bldr.Buildpacks()[0].ID, "buildpack-1-id")
				h.AssertEq(t, bldr.Buildpacks()[1].ID, "buildpack-2-id")

				order := bldr.Order()
				h.AssertEq(t, len(order), 1)
				h.AssertEq(t, len(order[0].Group), 2)
				h.AssertEq(t, order[0].Group[0].ID, "buildpack-1-id")
				h.AssertEq(t, order[0].Group[0].Version, "")
				h.AssertEq(t, order[0].Group[0].Optional, false)
				h.AssertEq(t, order[0].Group[1].ID, "buildpack-2-id")
				h.AssertEq(t, order[0].Group[1].Version, "buildpack-2-version-1")
				h.AssertEq(t, order[0].Group[1].Optional, true)
			})

			it("gets mixins from image", func() {
				h.AssertSliceContainsOnly(t, bldr.Mixins(), "mixinX", "mixinY", "build:mixinA")
			})

			when("metadata is missing", func() {
				it.Before(func() {
					h.AssertNil(t, builderImage.SetLabel(
						"io.buildpacks.builder.metadata",
						"",
					))
				})

				it("should error", func() {
					_, err := builder.FromImage(builderImage)
					h.AssertError(t, err, "missing label 'io.buildpacks.builder.metadata'")
				})
			})

			when("#Description", func() {
				it("return description", func() {
					h.AssertEq(t, bldr.Description(), "some-description")
				})
			})

			when("#CreatedBy", func() {
				it("return CreatedBy", func() {
					expectedCreatorMetadata := builder.CreatorMetadata{
						Name:    "some-name",
						Version: "1.2.3",
					}
					h.AssertEq(t, bldr.CreatedBy(), expectedCreatorMetadata)
				})
			})

			when("#Name", func() {
				it("return Name", func() {
					h.AssertEq(t, bldr.Name(), "base/image")
				})
			})

			when("#Image", func() {
				it("return Image", func() {
					h.AssertSameInstance(t, bldr.Image(), baseImage)
				})
			})

			when("#Stack", func() {
				it("return Stack", func() {
					expectedStack := builder.StackMetadata{
						RunImage: builder.RunImageMetadata{
							Image:   "prev/run",
							Mirrors: []string{"prev/mirror"}}}

					h.AssertEq(t, bldr.Stack(), expectedStack)
				})
			})

			when("#UID", func() {
				it("return UID", func() {
					h.AssertEq(t, bldr.UID(), 1234)
				})
			})

			when("#GID", func() {
				it("return GID", func() {
					h.AssertEq(t, bldr.GID(), 4321)
				})
			})
		})
	})
}

func assertImageHasBPLayer(t *testing.T, image *fakes.Image, bp dist.Buildpack) {
	t.Helper()

	dirPath := fmt.Sprintf("/cnb/buildpacks/%s/%s", bp.Descriptor().Info.ID, bp.Descriptor().Info.Version)
	layerTar, err := image.FindLayerWithPath(dirPath)
	h.AssertNil(t, err)

	h.AssertOnTarEntry(t, layerTar, dirPath,
		h.IsDirectory(),
	)

	h.AssertOnTarEntry(t, layerTar, path.Dir(dirPath),
		h.IsDirectory(),
	)

	h.AssertOnTarEntry(t, layerTar, dirPath+"/bin/build",
		h.ContentEquals("build-contents"),
	)

	h.AssertOnTarEntry(t, layerTar, dirPath+"/bin/detect",
		h.ContentEquals("detect-contents"),
	)
}

func assertImageHasOrderBpLayer(t *testing.T, image *fakes.Image, bp dist.Buildpack) {
	t.Helper()

	dirPath := fmt.Sprintf("/cnb/buildpacks/%s/%s", bp.Descriptor().Info.ID, bp.Descriptor().Info.Version)
	layerTar, err := image.FindLayerWithPath(dirPath)
	h.AssertNil(t, err)

	h.AssertOnTarEntry(t, layerTar, dirPath,
		h.IsDirectory(),
	)

	h.AssertOnTarEntry(t, layerTar, path.Dir(dirPath),
		h.IsDirectory(),
	)
}
