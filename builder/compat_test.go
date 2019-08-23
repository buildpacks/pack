package builder_test

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"runtime"
	"strings"
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

func TestCompat(t *testing.T) {
	color.NoColor = true
	spec.Run(t, "Compat", testCompat, spec.Parallel(), spec.Report(report.Terminal{}))
}

func testCompat(t *testing.T, when spec.G, it spec.S) {
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
			var buildpackTgz string

			it.Before(func() {
				err := os.Chmod(filepath.Join("testdata", "buildpack", "buildpack-file"), 0644)
				h.AssertNil(t, err)
				buildpackTgz = h.CreateTgz(t, filepath.Join("testdata", "buildpack"), "./", 0644)
			})

			it.After(func() {
				h.AssertNil(t, os.Remove(buildpackTgz))
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

			it("does not overwrite the compat order when SetOrder has not been called", func() {
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

			it("should create a compat lifecycle symlink", func() {
				layerTar, err := baseImage.FindLayerWithPath("/lifecycle")
				h.AssertNil(t, err)
				h.AssertOnTarEntry(t, layerTar, "/lifecycle", h.SymlinksTo("/cnb/lifecycle"))
			})
		})

		when("#AddBuildpack", func() {
			var buildpackTgz string

			it.Before(func() {
				err := os.Chmod(filepath.Join("testdata", "buildpack", "buildpack-file"), 0644)
				h.AssertNil(t, err)
				buildpackTgz = h.CreateTgz(t, filepath.Join("testdata", "buildpack"), "./", 0644)

				subject.AddBuildpack(buildpack.Buildpack{
					BuildpackInfo: buildpack.BuildpackInfo{
						ID:      "buildpack-1-id",
						Version: "buildpack-1-version",
					},
					Path: buildpackTgz,
					Order: buildpack.Order{
						{
							Group: []buildpack.BuildpackInfo{
								{
									ID:      "buildpack-2-id",
									Version: "buildpack-2-version",
								},
							},
						},
					},
				})

				subject.AddBuildpack(buildpack.Buildpack{
					BuildpackInfo: buildpack.BuildpackInfo{
						ID:      "buildpack-2-id",
						Version: "buildpack-2-version",
					},
					Path:   buildpackTgz,
					Stacks: []buildpack.Stack{{ID: "some.stack.id"}},
				})

				subject.AddBuildpack(buildpack.Buildpack{
					BuildpackInfo: buildpack.BuildpackInfo{
						ID:      "buildpack-2-id",
						Version: "buildpack-2-other-version",
					},
					Path:   buildpackTgz,
					Stacks: []buildpack.Stack{{ID: "some.stack.id"}},
				})

				subject.AddBuildpack(buildpack.Buildpack{
					BuildpackInfo: buildpack.BuildpackInfo{
						ID:      "buildpack-3-id",
						Version: "buildpack-3-version",
					},
					Path:   buildpackTgz,
					Stacks: []buildpack.Stack{{ID: "some.stack.id"}},
				})

				if runtime.GOOS != "windows" {
					subject.AddBuildpack(buildpack.Buildpack{
						BuildpackInfo: buildpack.BuildpackInfo{
							ID:      "dir-buildpack-id",
							Version: "dir-buildpack-version",
						},
						Path:   filepath.Join("testdata", "buildpack"),
						Stacks: []buildpack.Stack{{ID: "some.stack.id"}},
					})
				}
			})

			it.After(func() {
				h.AssertNil(t, os.Remove(buildpackTgz))
			})

			it("adds a compat symlink for each buildpack", func() {
				var (
					layerTar string
					err      error
				)

				h.AssertNil(t, subject.Save())
				h.AssertEq(t, baseImage.IsSaved(), true)

				layerTar, err = baseImage.FindLayerWithPath("/buildpacks/buildpack-1-id/buildpack-1-version")
				h.AssertNil(t, err)

				h.AssertOnTarEntry(t, layerTar, "/buildpacks/buildpack-1-id/buildpack-1-version",
					h.SymlinksTo("/cnb/buildpacks/buildpack-1-id/buildpack-1-version"),
				)

				h.AssertOnTarEntry(t, layerTar, "/buildpacks/buildpack-2-id/buildpack-2-version",
					h.SymlinksTo("/cnb/buildpacks/buildpack-2-id/buildpack-2-version"),
				)
			})

			when("lifecycle version is < 0.4.0", func() {
				var lifecycleTgz string

				it.Before(func() {
					lifecycleTgz = h.CreateTgz(t, filepath.Join("testdata", "lifecycle"), "./lifecycle", 0755)

					h.AssertNil(t, subject.SetLifecycle(lifecycle.Metadata{
						Version: semver.MustParse("0.3.9"),
						Path:    lifecycleTgz,
					}))

					h.AssertNil(t, subject.Save())
					h.AssertEq(t, baseImage.IsSaved(), true)
				})

				it.After(func() {
					h.AssertNil(t, os.Remove(lifecycleTgz))
				})

				it("adds latest symlinks", func() {
					layerTar, err := baseImage.FindLayerWithPath("/buildpacks/buildpack-1-id/buildpack-1-version")
					h.AssertNil(t, err)

					h.AssertOnTarEntry(t, layerTar, "/buildpacks/buildpack-1-id/latest",
						h.SymlinksTo("/cnb/buildpacks/buildpack-1-id/buildpack-1-version"),
					)
				})
			})

			when("lifecycle version is >= 0.4.0", func() {
				var lifecycleTgz string

				it.Before(func() {
					lifecycleTgz = h.CreateTgz(t, filepath.Join("testdata", "lifecycle"), "./lifecycle", 0755)

					h.AssertNil(t, subject.SetLifecycle(lifecycle.Metadata{
						Version: semver.MustParse("0.4.0"),
						Path:    lifecycleTgz,
					}))

					h.AssertNil(t, subject.Save())
					h.AssertEq(t, baseImage.IsSaved(), true)
				})

				it.After(func() {
					h.AssertNil(t, os.Remove(lifecycleTgz))
				})

				it("doesn't add the latest symlink", func() {
					layerTar, err := baseImage.FindLayerWithPath("/buildpacks/buildpack-1-id/buildpack-1-version")
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
		})

		when("#SetOrder", func() {
			when("the buildpacks exist in the image", func() {
				it.Before(func() {
					subject.AddBuildpack(buildpack.Buildpack{
						BuildpackInfo: buildpack.BuildpackInfo{
							ID:      "some-buildpack-id",
							Version: "some-buildpack-version",
						},
						Path:   filepath.Join("testdata", "buildpack"),
						Stacks: []buildpack.Stack{{ID: "some.stack.id"}},
					})
					subject.AddBuildpack(buildpack.Buildpack{
						BuildpackInfo: buildpack.BuildpackInfo{
							ID:      "optional-buildpack-id",
							Version: "older-optional-buildpack-version",
						},
						Path:   filepath.Join("testdata", "buildpack"),
						Stacks: []buildpack.Stack{{ID: "some.stack.id"}},
					})
					subject.AddBuildpack(buildpack.Buildpack{
						BuildpackInfo: buildpack.BuildpackInfo{
							ID:      "optional-buildpack-id",
							Version: "optional-buildpack-version",
						},
						Path:   filepath.Join("testdata", "buildpack"),
						Stacks: []buildpack.Stack{{ID: "some.stack.id"}},
					})
					subject.SetOrder(builder.Order{
						{Group: []builder.BuildpackRef{
							{
								BuildpackInfo: buildpack.BuildpackInfo{
									ID:      "some-buildpack-id",
									Version: "some-buildpack-version",
								},
							},
							{
								BuildpackInfo: buildpack.BuildpackInfo{
									ID:      "optional-buildpack-id",
									Version: "optional-buildpack-version",
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
					h.AssertOnTarEntry(t, layerTar, "/buildpacks/order.toml", h.ContentEquals(`[[order]]

  [[order.group]]
    id = "some-buildpack-id"
    version = "some-buildpack-version"

  [[order.group]]
    id = "optional-buildpack-id"
    version = "optional-buildpack-version"
    optional = true
`))
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

			it("adds a compat stack.toml to the image", func() {
				layerTar, err := baseImage.FindLayerWithPath("/buildpacks/stack.toml")
				h.AssertNil(t, err)
				h.AssertOnTarEntry(t, layerTar, "/buildpacks/stack.toml", h.ContentEquals(`[run-image]
  image = "some/run"
  mirrors = ["some/mirror", "other/mirror"]
`))
			})
		})
	})
}
