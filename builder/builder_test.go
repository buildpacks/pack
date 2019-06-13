package builder_test

import (
	"encoding/json"
	"io/ioutil"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/Masterminds/semver"
	"github.com/buildpack/imgutil/fakes"
	"github.com/fatih/color"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	"github.com/buildpack/pack/builder"
	"github.com/buildpack/pack/buildpack"
	"github.com/buildpack/pack/internal/archive"
	"github.com/buildpack/pack/lifecycle"
	h "github.com/buildpack/pack/testhelpers"
)

func TestBuilder(t *testing.T) {
	color.NoColor = true
	spec.Run(t, "Builder", testBuilder, spec.Parallel(), spec.Report(report.Terminal{}))
}

func testBuilder(t *testing.T, when spec.G, it spec.S) {
	var (
		baseImage *fakes.Image
		subject   *builder.Builder
	)

	it.Before(func() {
		baseImage = fakes.NewImage("base/image", "", "")
	})

	it.After(func() {
		baseImage.Cleanup()
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
					h.AssertError(t, err, "image 'base/image' missing 'io.buildpacks.stack.id' label")
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
		})

		it.After(func() {
			baseImage.Cleanup()
		})

		when("#Save", func() {
			it("creates a builder from the image and renames it", func() {
				h.AssertNil(t, subject.Save())
				h.AssertEq(t, baseImage.IsSaved(), true)
				h.AssertEq(t, baseImage.Name(), "some/builder")
			})

			it("creates the workspace dir with CNB user and group", func() {
				h.AssertNil(t, subject.Save())
				h.AssertEq(t, baseImage.IsSaved(), true)

				layerTar, err := baseImage.FindLayerWithPath("/workspace")
				h.AssertNil(t, err)
				h.AssertOnTarEntry(t, layerTar, "/workspace",
					h.IsDirectory(),
					h.HasFileMode(0755),
					h.HasOwnerAndGroup(1234, 4321),
				)
			})

			it("creates the layers dir with CNB user and group", func() {
				h.AssertNil(t, subject.Save())
				h.AssertEq(t, baseImage.IsSaved(), true)

				layerTar, err := baseImage.FindLayerWithPath("/layers")
				h.AssertNil(t, err)
				h.AssertOnTarEntry(t, layerTar, "/layers",
					h.IsDirectory(),
					h.HasOwnerAndGroup(1234, 4321),
					h.HasFileMode(0755),
				)
			})

			it("creates the buildpacks dir", func() {
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

			it("creates the platform dir", func() {
				h.AssertNil(t, subject.Save())
				h.AssertEq(t, baseImage.IsSaved(), true)

				layerTar, err := baseImage.FindLayerWithPath("/platform")
				h.AssertNil(t, err)
				h.AssertOnTarEntry(t, layerTar, "/platform",
					h.IsDirectory(),
					h.HasOwnerAndGroup(0, 0),
					h.HasFileMode(0755),
				)
			})

			it("sets the working dir to the layers dir", func() {
				h.AssertNil(t, subject.Save())
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

				err = archive.CreateSingleFileTar(f.Name(), "/buildpacks/order.toml", "some content")
				h.AssertNil(t, err)

				h.AssertNil(t, baseImage.AddLayer(layerFile))
				baseImage.Save()

				h.AssertNil(t, subject.Save())
				h.AssertEq(t, baseImage.IsSaved(), true)

				layerTar, err := baseImage.FindLayerWithPath("/buildpacks/order.toml")
				h.AssertNil(t, err)
				h.AssertOnTarEntry(t, layerTar, "/buildpacks/order.toml", h.ContentEquals("some content"))
			})

		})

		when("#SetLifecycle", func() {
			var lifecycleTgz string

			it.Before(func() {
				lifecycleTgz = h.CreateTgz(t, filepath.Join("testdata", "lifecycle"), "./lifecycle", 0755)

				h.AssertNil(t, subject.SetLifecycle(lifecycle.Metadata{
					Version: semver.MustParse("1.2.3"),
					Path:    lifecycleTgz,
				}))
				h.AssertNil(t, subject.Save())
				h.AssertEq(t, baseImage.IsSaved(), true)
			})

			it.After(func() {
				h.AssertNil(t, os.Remove(lifecycleTgz))
			})

			it("should set the lifecycle version successfully", func() {
				h.AssertEq(t, subject.GetLifecycleVersion().String(), "1.2.3")
			})

			it("should add the lifecycle binaries as an image layer", func() {
				layerTar, err := baseImage.FindLayerWithPath("/lifecycle")
				h.AssertNil(t, err)
				h.AssertOnTarEntry(t, layerTar, "/lifecycle",
					h.IsDirectory(),
					h.HasFileMode(0755),
				)

				h.AssertOnTarEntry(t, layerTar, "/lifecycle/detector",
					h.ContentEquals("detector"),
					h.HasFileMode(0755),
				)

				h.AssertOnTarEntry(t, layerTar, "/lifecycle/restorer",
					h.ContentEquals("restorer"),
					h.HasFileMode(0755),
				)

				h.AssertOnTarEntry(t, layerTar, "/lifecycle/analyzer",
					h.ContentEquals("analyzer"),
					h.HasFileMode(0755),
				)

				h.AssertOnTarEntry(t, layerTar, "/lifecycle/builder",
					h.ContentEquals("builder"),
					h.HasFileMode(0755),
				)

				h.AssertOnTarEntry(t, layerTar, "/lifecycle/exporter",
					h.ContentEquals("exporter"),
					h.HasFileMode(0755),
				)

				h.AssertOnTarEntry(t, layerTar, "/lifecycle/cacher",
					h.ContentEquals("cacher"),
					h.HasFileMode(0755),
				)

				h.AssertOnTarEntry(t, layerTar, "/lifecycle/launcher",
					h.ContentEquals("launcher"),
					h.HasFileMode(0755),
				)
			})

			it("sets the lifecycle version on the metadata", func() {
				label, err := baseImage.Label("io.buildpacks.builder.metadata")
				h.AssertNil(t, err)

				var metadata builder.Metadata
				h.AssertNil(t, json.Unmarshal([]byte(label), &metadata))
				h.AssertEq(t, metadata.Lifecycle.Version.String(), "1.2.3")
			})
		})

		when("#AddBuildpack", func() {
			it.Before(func() {
				err := os.Chmod(filepath.Join("testdata", "buildpack", "buildpack-file"), 0644)
				h.AssertNil(t, err)
			})

			when("buildpack has matching stack", func() {
				var buildpackTgz string

				it.Before(func() {
					buildpackTgz = h.CreateTgz(t, filepath.Join("testdata", "buildpack"), "./", 0644)

					h.AssertNil(t, subject.AddBuildpack(buildpack.Buildpack{
						ID:      "tgz-buildpack-id",
						Version: "tgz-buildpack-version",
						Path:    buildpackTgz,
						Latest:  false,
						Stacks:  []buildpack.Stack{{ID: "some.stack.id"}},
					}))

					h.AssertNil(t, subject.AddBuildpack(buildpack.Buildpack{
						ID:      "latest-buildpack-id",
						Version: "latest-buildpack-version",
						Path:    buildpackTgz,
						Latest:  true,
						Stacks:  []buildpack.Stack{{ID: "some.stack.id"}},
					}))

					if runtime.GOOS != "windows" {
						h.AssertNil(t, subject.AddBuildpack(buildpack.Buildpack{
							ID:      "dir-buildpack-id",
							Version: "dir-buildpack-version",
							Path:    filepath.Join("testdata", "buildpack"),
							Stacks:  []buildpack.Stack{{ID: "some.stack.id"}},
						}))
					}

					h.AssertNil(t, subject.Save())
					h.AssertEq(t, baseImage.IsSaved(), true)
				})

				it.After(func() {
					h.AssertNil(t, os.Remove(buildpackTgz))
				})

				it("adds the buildpack as an image layer", func() {
					var (
						layerTar string
						err      error
					)

					layerTar, err = baseImage.FindLayerWithPath("/buildpacks/tgz-buildpack-id/tgz-buildpack-version")
					h.AssertNil(t, err)

					h.AssertOnTarEntry(t, layerTar, "/buildpacks/tgz-buildpack-id/tgz-buildpack-version",
						h.IsDirectory(),
					)

					h.AssertOnTarEntry(t, layerTar, "/buildpacks/tgz-buildpack-id/tgz-buildpack-version/buildpack-file",
						h.ContentEquals("buildpack-contents"),
						h.HasOwnerAndGroup(1234, 4321),
						h.HasFileMode(0644),
					)

					layerTar, err = baseImage.FindLayerWithPath("/buildpacks/latest-buildpack-id/latest-buildpack-version")
					h.AssertNil(t, err)
					h.AssertOnTarEntry(t, layerTar, "/buildpacks/latest-buildpack-id/latest-buildpack-version",
						h.IsDirectory(),
					)

					h.AssertOnTarEntry(t, layerTar, "/buildpacks/latest-buildpack-id/latest-buildpack-version/buildpack-file",
						h.ContentEquals("buildpack-contents"),
						h.HasOwnerAndGroup(1234, 4321),
						h.HasFileMode(0644),
					)

					if runtime.GOOS != "windows" {
						layerTar, err = baseImage.FindLayerWithPath("/buildpacks/dir-buildpack-id/dir-buildpack-version")
						h.AssertNil(t, err)
						h.AssertOnTarEntry(t, layerTar, "/buildpacks/dir-buildpack-id/dir-buildpack-version",
							h.IsDirectory(),
						)

						h.AssertOnTarEntry(t, layerTar, "/buildpacks/dir-buildpack-id/dir-buildpack-version/buildpack-file",
							h.ContentEquals("buildpack-contents"),
							h.HasOwnerAndGroup(1234, 4321),
							h.HasFileMode(0644),
						)
					}
				})

				it("adds a symlink to the buildpack layer if latest is true", func() {
					layerTar, err := baseImage.FindLayerWithPath("/buildpacks/latest-buildpack-id")
					h.AssertNil(t, err)

					h.AssertOnTarEntry(t,
						layerTar,
						"/buildpacks/latest-buildpack-id/latest",
						h.SymlinksTo("/buildpacks/latest-buildpack-id/latest-buildpack-version"),
						h.HasOwnerAndGroup(0, 0),
						h.HasFileMode(0644),
					)
				})

				it("adds the buildpack metadata", func() {
					label, err := baseImage.Label("io.buildpacks.builder.metadata")
					h.AssertNil(t, err)

					var metadata builder.Metadata
					h.AssertNil(t, json.Unmarshal([]byte(label), &metadata))
					if runtime.GOOS == "windows" {
						h.AssertEq(t, len(metadata.Buildpacks), 2)
					} else {
						h.AssertEq(t, len(metadata.Buildpacks), 3)
					}

					h.AssertEq(t, metadata.Buildpacks[0].ID, "tgz-buildpack-id")
					h.AssertEq(t, metadata.Buildpacks[0].Version, "tgz-buildpack-version")
					h.AssertEq(t, metadata.Buildpacks[0].Latest, false)

					h.AssertEq(t, metadata.Buildpacks[1].ID, "latest-buildpack-id")
					h.AssertEq(t, metadata.Buildpacks[1].Version, "latest-buildpack-version")
					h.AssertEq(t, metadata.Buildpacks[1].Latest, true)

					if runtime.GOOS != "windows" {
						h.AssertEq(t, metadata.Buildpacks[2].ID, "dir-buildpack-id")
						h.AssertEq(t, metadata.Buildpacks[2].Version, "dir-buildpack-version")
						h.AssertEq(t, metadata.Buildpacks[2].Latest, false)
					}
				})
			})

			when("buildpack stack id does not match", func() {
				it("returns an error", func() {
					err := subject.AddBuildpack(buildpack.Buildpack{
						ID:      "some-buildpack-id",
						Version: "some-buildpack-version",
						Path:    filepath.Join("testdata", "buildpack"),
						Stacks:  []buildpack.Stack{{ID: "other.stack.id"}},
					})
					h.AssertError(t, err, "buildpack 'some-buildpack-id' version 'some-buildpack-version' does not support stack 'some.stack.id'")
				})
			})

			when("base image already has metadata", func() {
				it.Before(func() {
					h.AssertNil(t, baseImage.SetLabel(
						"io.buildpacks.builder.metadata",
						`{"buildpacks": [{"id": "prev.id"}], "groups": [{"buildpacks": [{"id": "prev.id"}]}], "stack": {"runImage": {"image": "prev/run", "mirrors": ["prev/mirror"]}}, "lifecycle": {"version": "6.6.6"}}`,
					))

					var err error
					subject, err = builder.New(baseImage, "some/builder")
					h.AssertNil(t, err)

					h.AssertNil(t, subject.AddBuildpack(buildpack.Buildpack{
						ID:      "some-buildpack-id",
						Version: "some-buildpack-version",
						Path:    filepath.Join("testdata", "buildpack"),
						Stacks:  []buildpack.Stack{{ID: "some.stack.id"}},
					}))
					h.AssertNil(t, subject.Save())
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
					h.AssertEq(t, metadata.Groups[0].Buildpacks[0].ID, "prev.id")
					h.AssertEq(t, metadata.Stack.RunImage.Image, "prev/run")
					h.AssertEq(t, metadata.Stack.RunImage.Mirrors[0], "prev/mirror")
					h.AssertEq(t, subject.GetLifecycleVersion().String(), "6.6.6")

					// adds new buildpack
					h.AssertEq(t, metadata.Buildpacks[1].ID, "some-buildpack-id")
					h.AssertEq(t, metadata.Buildpacks[1].Version, "some-buildpack-version")
					h.AssertEq(t, metadata.Buildpacks[1].Latest, false)
				})
			})
		})

		when("#SetOrder", func() {
			when("the buildpacks exist in the image", func() {
				it.Before(func() {
					h.AssertNil(t, subject.AddBuildpack(buildpack.Buildpack{
						ID:      "some-buildpack-id",
						Version: "some-buildpack-version",
						Path:    filepath.Join("testdata", "buildpack"),
						Stacks:  []buildpack.Stack{{ID: "some.stack.id"}},
					}))
					h.AssertNil(t, subject.AddBuildpack(buildpack.Buildpack{
						ID:      "optional-buildpack-id",
						Version: "older-optional-buildpack-version",
						Path:    filepath.Join("testdata", "buildpack"),
						Stacks:  []buildpack.Stack{{ID: "some.stack.id"}},
					}))
					h.AssertNil(t, subject.AddBuildpack(buildpack.Buildpack{
						ID:      "optional-buildpack-id",
						Version: "optional-buildpack-version",
						Latest:  true,
						Path:    filepath.Join("testdata", "buildpack"),
						Stacks:  []buildpack.Stack{{ID: "some.stack.id"}},
					}))
					h.AssertNil(t, subject.SetOrder([]builder.GroupMetadata{
						{Buildpacks: []builder.GroupBuildpack{
							{
								ID:      "some-buildpack-id",
								Version: "some-buildpack-version",
							},
							{
								ID:       "optional-buildpack-id",
								Version:  "latest",
								Optional: true,
							},
						}},
					}))
					h.AssertNil(t, subject.Save())
					h.AssertEq(t, baseImage.IsSaved(), true)
				})

				it("adds the order.toml to the image", func() {
					layerTar, err := baseImage.FindLayerWithPath("/buildpacks/order.toml")
					h.AssertNil(t, err)
					h.AssertOnTarEntry(t, layerTar, "/buildpacks/order.toml", h.ContentEquals(`[[groups]]

  [[groups.buildpacks]]
    id = "some-buildpack-id"
    version = "some-buildpack-version"

  [[groups.buildpacks]]
    id = "optional-buildpack-id"
    version = "latest"
    optional = true
`))
				})

				it("adds the order to the metadata", func() {
					label, err := baseImage.Label("io.buildpacks.builder.metadata")
					h.AssertNil(t, err)

					var metadata builder.Metadata
					h.AssertNil(t, json.Unmarshal([]byte(label), &metadata))

					h.AssertEq(t, len(metadata.Groups), 1)
					h.AssertEq(t, len(metadata.Groups[0].Buildpacks), 2)

					h.AssertEq(t, metadata.Groups[0].Buildpacks[0].ID, "some-buildpack-id")
					h.AssertEq(t, metadata.Groups[0].Buildpacks[0].Version, "some-buildpack-version")

					h.AssertEq(t, metadata.Groups[0].Buildpacks[1].ID, "optional-buildpack-id")
					h.AssertEq(t, metadata.Groups[0].Buildpacks[1].Version, "latest")
					h.AssertEq(t, metadata.Groups[0].Buildpacks[1].Optional, true)
				})

				when("the group buildpack has latest version", func() {
					it("fails if no buildpack is tagged as latest", func() {
						err := subject.SetOrder([]builder.GroupMetadata{
							{Buildpacks: []builder.GroupBuildpack{
								{
									ID:      "some-buildpack-id",
									Version: "latest",
								},
							}},
						})
						h.AssertError(t, err, "there is no version of buildpack 'some-buildpack-id' marked as latest")
					})
				})
			})

			when("no version of the group buildpack exists in the image", func() {
				it("errors", func() {
					err := subject.SetOrder([]builder.GroupMetadata{
						{Buildpacks: []builder.GroupBuildpack{
							{
								ID:      "some-buildpack-id",
								Version: "some-buildpack-version",
							},
						}},
					})
					h.AssertError(t, err, "no versions of buildpack 'some-buildpack-id' were found on the builder")
				})
			})

			when("wrong versions of the group buildpack exists in the image", func() {
				it("errors", func() {
					h.AssertNil(t, subject.AddBuildpack(buildpack.Buildpack{
						ID:      "some-buildpack-id",
						Version: "some-buildpack-version",
						Path:    filepath.Join("testdata", "buildpack"),
						Stacks:  []buildpack.Stack{{ID: "some.stack.id"}},
					}))
					err := subject.SetOrder([]builder.GroupMetadata{
						{Buildpacks: []builder.GroupBuildpack{
							{
								ID:      "some-buildpack-id",
								Version: "wrong-version",
							},
						}},
					})
					h.AssertError(t, err, "buildpack 'some-buildpack-id' with version 'wrong-version' was not found on the builder")
				})
			})
		})

		when("#SetDescription", func() {
			it.Before(func() {
				subject.SetDescription("Some description")
				h.AssertNil(t, subject.Save())
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
				h.AssertNil(t, subject.Save())
				h.AssertEq(t, baseImage.IsSaved(), true)
			})

			it("adds the stack.toml to the image", func() {
				layerTar, err := baseImage.FindLayerWithPath("/buildpacks/stack.toml")
				h.AssertNil(t, err)
				h.AssertOnTarEntry(t, layerTar, "/buildpacks/stack.toml", h.ContentEquals(`[run-image]
  image = "some/run"
  mirrors = ["some/mirror", "other/mirror"]
`))
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
				h.AssertNil(t, subject.Save())
				h.AssertEq(t, baseImage.IsSaved(), true)
			})

			it("adds the env vars as files to the image", func() {
				layerTar, err := baseImage.FindLayerWithPath("/platform/env/SOME_KEY")
				h.AssertNil(t, err)
				h.AssertOnTarEntry(t, layerTar, "/platform/env/SOME_KEY", h.ContentEquals(`some-val`))
				h.AssertOnTarEntry(t, layerTar, "/platform/env/OTHER_KEY", h.ContentEquals(`other-val`))
			})
		})
	})
}
