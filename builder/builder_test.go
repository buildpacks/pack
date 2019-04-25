package builder_test

import (
	"archive/tar"
	"encoding/json"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/buildpack/imgutil/fakes"
	"github.com/fatih/color"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	"github.com/buildpack/pack/builder"
	"github.com/buildpack/pack/buildpack"

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
		})

		when("#AddBuildpack", func() {
			when("buildpack has matching stack", func() {
				it.Before(func() {
					h.AssertNil(t, subject.AddBuildpack(buildpack.Buildpack{
						ID:      "some-buildpack-id",
						Version: "some-buildpack-version",
						Dir:     filepath.Join("testdata", "buildpack"),
						Stacks:  []buildpack.Stack{{ID: "some.stack.id"}},
					}))

					h.AssertNil(t, subject.AddBuildpack(buildpack.Buildpack{
						ID:      "other-buildpack-id",
						Version: "other-buildpack-version",
						Dir:     filepath.Join("testdata", "buildpack"),
						Latest:  true,
						Stacks:  []buildpack.Stack{{ID: "some.stack.id"}},
					}))

					h.AssertNil(t, subject.Save())
					h.AssertEq(t, baseImage.IsSaved(), true)
				})

				it("adds the buildpack as an image layer", func() {
					layerTar, err := baseImage.FindLayerWithPath("/buildpacks/some-buildpack-id/some-buildpack-version")
					h.AssertNil(t, err)
					assertTarFileContents(t, layerTar, "/buildpacks/some-buildpack-id/some-buildpack-version/buildpack-file", "buildpack-contents")

					layerTar, err = baseImage.FindLayerWithPath("/buildpacks/other-buildpack-id/other-buildpack-version")
					h.AssertNil(t, err)
					assertTarFileContents(t, layerTar, "/buildpacks/other-buildpack-id/other-buildpack-version/buildpack-file", "buildpack-contents")
				})

				it("adds a symlink to the buildpack layer if latest is true", func() {
					layerTar, err := baseImage.FindLayerWithPath("/buildpacks/other-buildpack-id")
					h.AssertNil(t, err)
					assertTarFileSymlink(t, layerTar, "/buildpacks/other-buildpack-id/latest", "/buildpacks/other-buildpack-id/other-buildpack-version")
					assertTarFileOwner(t, layerTar, "/buildpacks/other-buildpack-id/latest", 1234, 4321)
				})

				it("adds the buildpack contents with the correct uid and gid", func() {
					layerTar, err := baseImage.FindLayerWithPath("/buildpacks/some-buildpack-id/some-buildpack-version")
					h.AssertNil(t, err)
					assertTarFileOwner(t, layerTar, "/buildpacks/some-buildpack-id/some-buildpack-version/buildpack-file", 1234, 4321)

					layerTar, err = baseImage.FindLayerWithPath("/buildpacks/other-buildpack-id/other-buildpack-version")
					h.AssertNil(t, err)
					assertTarFileOwner(t, layerTar, "/buildpacks/other-buildpack-id/other-buildpack-version/buildpack-file", 1234, 4321)
				})

				it("adds the buildpack metadata", func() {
					label, err := baseImage.Label("io.buildpacks.builder.metadata")
					h.AssertNil(t, err)

					var metadata builder.Metadata
					h.AssertNil(t, json.Unmarshal([]byte(label), &metadata))
					h.AssertEq(t, len(metadata.Buildpacks), 2)

					h.AssertEq(t, metadata.Buildpacks[0].ID, "some-buildpack-id")
					h.AssertEq(t, metadata.Buildpacks[0].Version, "some-buildpack-version")
					h.AssertEq(t, metadata.Buildpacks[0].Latest, false)

					h.AssertEq(t, metadata.Buildpacks[1].ID, "other-buildpack-id")
					h.AssertEq(t, metadata.Buildpacks[1].Version, "other-buildpack-version")
					h.AssertEq(t, metadata.Buildpacks[1].Latest, true)
				})
			})

			when("buildpack stack id does not match", func() {
				it("returns an error", func() {
					err := subject.AddBuildpack(buildpack.Buildpack{
						ID:      "some-buildpack-id",
						Version: "some-buildpack-version",
						Dir:     filepath.Join("testdata", "buildpack"),
						Stacks:  []buildpack.Stack{{ID: "other.stack.id"}},
					})
					h.AssertError(t, err, "buildpack 'some-buildpack-id' version 'some-buildpack-version' does not support stack 'some.stack.id'")
				})
			})

			when("base image already has metadata", func() {
				it.Before(func() {
					h.AssertNil(t, baseImage.SetLabel("io.buildpacks.builder.metadata", `{"buildpacks": [{"id": "prev.id"}], "groups": [{"buildpacks": [{"id": "prev.id"}]}], "stack": {"runImage": {"image": "prev/run", "mirrors": ["prev/mirror"]}}}`))

					var err error
					subject, err = builder.New(baseImage, "some/builder")
					h.AssertNil(t, err)

					h.AssertNil(t, subject.AddBuildpack(buildpack.Buildpack{
						ID:      "some-buildpack-id",
						Version: "some-buildpack-version",
						Dir:     filepath.Join("testdata", "buildpack"),
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
						Dir:     filepath.Join("testdata", "buildpack"),
						Stacks:  []buildpack.Stack{{ID: "some.stack.id"}},
					}))
					h.AssertNil(t, subject.AddBuildpack(buildpack.Buildpack{
						ID:      "optional-buildpack-id",
						Version: "older-optional-buildpack-version",
						Dir:     filepath.Join("testdata", "buildpack"),
						Stacks:  []buildpack.Stack{{ID: "some.stack.id"}},
					}))
					h.AssertNil(t, subject.AddBuildpack(buildpack.Buildpack{
						ID:      "optional-buildpack-id",
						Version: "optional-buildpack-version",
						Latest:  true,
						Dir:     filepath.Join("testdata", "buildpack"),
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
					assertTarFileContents(t, layerTar, "/buildpacks/order.toml", `[[groups]]

  [[groups.buildpacks]]
    id = "some-buildpack-id"
    version = "some-buildpack-version"

  [[groups.buildpacks]]
    id = "optional-buildpack-id"
    version = "latest"
    optional = true
`)
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
						Dir:     filepath.Join("testdata", "buildpack"),
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
				assertTarFileContents(t, layerTar, "/buildpacks/stack.toml", `[run-image]
  image = "some/run"
  mirrors = ["some/mirror", "other/mirror"]
`)
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
				layerTar, err := baseImage.FindLayerWithPath("/platform/env")
				h.AssertNil(t, err)
				assertTarFileContents(t, layerTar, "/platform/env/SOME_KEY", `some-val`)
				assertTarFileContents(t, layerTar, "/platform/env/OTHER_KEY", `other-val`)
			})
		})
	})
}

func assertTarFileContents(t *testing.T, tarfile, path, expected string) {
	t.Helper()
	exist, contents := tarFileContents(t, tarfile, path)
	if !exist {
		t.Fatalf("%s does not exist in %s", path, tarfile)
	}
	h.AssertEq(t, contents, expected)
}

func assertTarFileSymlink(t *testing.T, tarFile, path, expected string) {
	t.Helper()
	r, err := os.Open(tarFile)
	h.AssertNil(t, err)
	defer r.Close()

	tr := tar.NewReader(r)
	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		h.AssertNil(t, err)

		if header.Name != path {
			continue
		}

		if header.Typeflag != tar.TypeSymlink {
			t.Fatalf("path '%s' is not a symlink, type flag is '%c'", header.Name, header.Typeflag)
		}

		if header.Linkname != expected {
			t.Fatalf("symlink '%s' does not point to '%s', instead it points to '%s'", header.Name, expected, header.Linkname)
		}
	}
}

func tarFileContents(t *testing.T, tarfile, path string) (exist bool, contents string) {
	t.Helper()
	r, err := os.Open(tarfile)
	h.AssertNil(t, err)
	defer r.Close()

	tr := tar.NewReader(r)
	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		h.AssertNil(t, err)

		if header.Name == path {
			buf, err := ioutil.ReadAll(tr)
			h.AssertNil(t, err)
			return true, string(buf)
		}
	}
	return false, ""
}

func assertTarFileOwner(t *testing.T, tarfile, path string, expectedUID, expectedGID int) {
	t.Helper()
	var foundPath bool
	r, err := os.Open(tarfile)
	h.AssertNil(t, err)
	defer r.Close()

	tr := tar.NewReader(r)
	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		h.AssertNil(t, err)

		if header.Name == path {
			foundPath = true
			if header.Uid != expectedUID {
				t.Fatalf("expected all entries in `%s` to have uid '%d', but '%s' has '%d'", tarfile, expectedUID, header.Name, header.Uid)
			}
			if header.Gid != expectedGID {
				t.Fatalf("expected all entries in `%s` to have gid '%d', got '%d'", tarfile, expectedGID, header.Gid)
			}
		}
	}
	if !foundPath {
		t.Fatalf("%s does not exist in %s", path, tarfile)
	}
}
