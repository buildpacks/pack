package pack

import (
	"archive/tar"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"io/ioutil"
	"math/rand"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/Masterminds/semver"
	"github.com/buildpacks/imgutil/fakes"
	"github.com/docker/docker/client"
	"github.com/heroku/color"
	"github.com/onsi/gomega/ghttp"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	"github.com/buildpacks/pack/internal/api"
	"github.com/buildpacks/pack/internal/blob"
	"github.com/buildpacks/pack/internal/builder"
	"github.com/buildpacks/pack/internal/buildpackage"
	"github.com/buildpacks/pack/internal/dist"
	ifakes "github.com/buildpacks/pack/internal/fakes"
	ilogging "github.com/buildpacks/pack/internal/logging"
	"github.com/buildpacks/pack/internal/style"
	"github.com/buildpacks/pack/logging"
	h "github.com/buildpacks/pack/testhelpers"
)

func TestBuild(t *testing.T) {
	color.Disable(true)
	defer color.Disable(false)
	rand.Seed(time.Now().UTC().UnixNano())
	spec.Run(t, "build", testBuild, spec.Report(report.Terminal{}))
}

func testBuild(t *testing.T, when spec.G, it spec.S) {
	var (
		subject               *Client
		fakeImageFetcher      *ifakes.FakeImageFetcher
		fakeLifecycle         *ifakes.FakeLifecycle
		defaultBuilderStackID string
		defaultBuilderImage   *fakes.Image
		builderName           string
		fakeDefaultRunImage   *fakes.Image
		fakeMirror1           *fakes.Image
		fakeMirror2           *fakes.Image
		tmpDir                string
		outBuf                bytes.Buffer
		logger                logging.Logger
	)
	it.Before(func() {
		var err error

		fakeImageFetcher = ifakes.NewFakeImageFetcher()
		fakeLifecycle = &ifakes.FakeLifecycle{}

		tmpDir, err = ioutil.TempDir("", "build-test")
		h.AssertNil(t, err)

		builderName = "example.com/default/builder:tag"
		defaultBuilderStackID = "some.stack.id"
		defaultBuilderImage = ifakes.NewFakeBuilderImage(t,
			tmpDir,
			builderName,
			defaultBuilderStackID,
			"1234",
			"5678",
			builder.Metadata{
				Buildpacks: []dist.BuildpackInfo{
					{ID: "buildpack.1.id", Version: "buildpack.1.version"},
					{ID: "buildpack.2.id", Version: "buildpack.2.version"},
				},
				Stack: builder.StackMetadata{
					RunImage: builder.RunImageMetadata{
						Image: "default/run",
						Mirrors: []string{
							"registry1.example.com/run/mirror",
							"registry2.example.com/run/mirror",
						},
					},
				},
				Lifecycle: builder.LifecycleMetadata{
					LifecycleInfo: builder.LifecycleInfo{
						Version: &builder.Version{
							Version: *semver.MustParse("0.3.0"),
						},
					},
					API: builder.LifecycleAPI{
						BuildpackVersion: api.MustParse("0.3"),
						PlatformVersion:  api.MustParse("0.2"),
					},
				},
			},
			dist.BuildpackLayers{
				"buildpack.1.id": {
					"buildpack.1.version": {
						API: api.MustParse("0.3"),
						Stacks: []dist.Stack{
							{
								ID:     defaultBuilderStackID,
								Mixins: []string{"mixinX", "build:mixinY", "run:mixinZ"},
							},
						},
					},
				},
				"buildpack.2.id": {
					"buildpack.2.version": {
						API: api.MustParse("0.3"),
						Stacks: []dist.Stack{
							{
								ID:     defaultBuilderStackID,
								Mixins: []string{"mixinX", "build:mixinY"},
							},
						},
					},
				},
			},
			dist.Order{{
				Group: []dist.BuildpackRef{{
					BuildpackInfo: dist.BuildpackInfo{
						ID:      "buildpack.1.id",
						Version: "buildpack.1.version",
					},
				}},
			}, {
				Group: []dist.BuildpackRef{{
					BuildpackInfo: dist.BuildpackInfo{
						ID:      "buildpack.2.id",
						Version: "buildpack.2.version",
					},
				}},
			}},
		)
		h.AssertNil(t, defaultBuilderImage.SetLabel("io.buildpacks.stack.mixins", `["mixinA", "build:mixinB", "mixinX", "build:mixinY"]`))
		fakeImageFetcher.LocalImages[defaultBuilderImage.Name()] = defaultBuilderImage

		fakeDefaultRunImage = fakes.NewImage("default/run", "", nil)
		h.AssertNil(t, fakeDefaultRunImage.SetLabel("io.buildpacks.stack.id", defaultBuilderStackID))
		h.AssertNil(t, fakeDefaultRunImage.SetLabel("io.buildpacks.stack.mixins", `["mixinA", "run:mixinC", "mixinX", "run:mixinZ"]`))
		fakeImageFetcher.LocalImages[fakeDefaultRunImage.Name()] = fakeDefaultRunImage

		fakeMirror1 = fakes.NewImage("registry1.example.com/run/mirror", "", nil)
		h.AssertNil(t, fakeMirror1.SetLabel("io.buildpacks.stack.id", defaultBuilderStackID))
		h.AssertNil(t, fakeMirror1.SetLabel("io.buildpacks.stack.mixins", `["mixinA", "mixinX", "run:mixinZ"]`))
		fakeImageFetcher.LocalImages[fakeMirror1.Name()] = fakeMirror1

		fakeMirror2 = fakes.NewImage("registry2.example.com/run/mirror", "", nil)
		h.AssertNil(t, fakeMirror2.SetLabel("io.buildpacks.stack.id", defaultBuilderStackID))
		h.AssertNil(t, fakeMirror2.SetLabel("io.buildpacks.stack.mixins", `["mixinA", "mixinX", "run:mixinZ"]`))
		fakeImageFetcher.LocalImages[fakeMirror2.Name()] = fakeMirror2

		docker, err := client.NewClientWithOpts(client.FromEnv, client.WithVersion("1.38"))
		h.AssertNil(t, err)

		logger = ilogging.NewLogWithWriters(&outBuf, &outBuf)

		dlCacheDir, err := ioutil.TempDir(tmpDir, "dl-cache")
		h.AssertNil(t, err)

		subject = &Client{
			logger:       logger,
			imageFetcher: fakeImageFetcher,
			downloader:   blob.NewDownloader(logger, dlCacheDir),
			lifecycle:    fakeLifecycle,
			docker:       docker,
		}
	})

	it.After(func() {
		defaultBuilderImage.Cleanup()
		fakeDefaultRunImage.Cleanup()
		fakeMirror1.Cleanup()
		fakeMirror2.Cleanup()
		os.RemoveAll(tmpDir)
	})

	when("#Build", func() {
		when("Image option", func() {
			it("is required", func() {
				h.AssertError(t, subject.Build(context.TODO(), BuildOptions{
					Image:   "",
					Builder: builderName,
				}),
					"invalid image name ''",
				)
			})

			it("must be a valid image reference", func() {
				h.AssertError(t, subject.Build(context.TODO(), BuildOptions{
					Image:   "not@valid",
					Builder: builderName,
				}),
					"invalid image name 'not@valid'",
				)
			})

			it("must be a valid tag reference", func() {
				h.AssertError(t, subject.Build(context.TODO(), BuildOptions{
					Image:   "registry.com/my/image@sha256:954e1f01e80ce09d0887ff6ea10b13a812cb01932a0781d6b0cc23f743a874fd",
					Builder: builderName,
				}),
					"invalid image name 'registry.com/my/image@sha256:954e1f01e80ce09d0887ff6ea10b13a812cb01932a0781d6b0cc23f743a874fd'",
				)
			})

			it("lifecycle receives resolved reference", func() {
				h.AssertNil(t, subject.Build(context.TODO(), BuildOptions{
					Builder: builderName,
					Image:   "example.com/some/repo:tag",
				}))
				h.AssertEq(t, fakeLifecycle.Opts.Image.Context().RegistryStr(), "example.com")
				h.AssertEq(t, fakeLifecycle.Opts.Image.Context().RepositoryStr(), "some/repo")
				h.AssertEq(t, fakeLifecycle.Opts.Image.Identifier(), "tag")
			})
		})

		when("AppDir option", func() {
			it("defaults to the current working directory", func() {
				h.AssertNil(t, subject.Build(context.TODO(), BuildOptions{
					Image:   "some/app",
					Builder: builderName,
				}))

				wd, err := os.Getwd()
				h.AssertNil(t, err)
				resolvedWd, err := filepath.EvalSymlinks(wd)
				h.AssertNil(t, err)
				h.AssertEq(t, fakeLifecycle.Opts.AppPath, resolvedWd)
			})
			for fileDesc, appPath := range map[string]string{
				"zip": filepath.Join("testdata", "zip-file.zip"),
				"jar": filepath.Join("testdata", "jar-file.jar"),
			} {
				fileDesc := fileDesc
				appPath := appPath

				it(fmt.Sprintf("supports %s files", fileDesc), func() {
					err := subject.Build(context.TODO(), BuildOptions{
						Image:   "some/app",
						Builder: builderName,
						AppPath: appPath,
					})
					h.AssertNil(t, err)
				})
			}

			for fileDesc, testData := range map[string][]string{
				"non-existent": {"not/exist/path", "does not exist"},
				"empty":        {filepath.Join("testdata", "empty-file"), "app path must be a directory or zip"},
				"non-zip":      {filepath.Join("testdata", "non-zip-file"), "app path must be a directory or zip"},
			} {
				fileDesc := fileDesc
				appPath := testData[0]
				errMessage := testData[0]

				it(fmt.Sprintf("does NOT support %s files", fileDesc), func() {
					err := subject.Build(context.TODO(), BuildOptions{
						Image:   "some/app",
						Builder: builderName,
						AppPath: appPath,
					})

					h.AssertError(t, err, errMessage)
				})
			}

			it("resolves the absolute path", func() {
				h.AssertNil(t, subject.Build(context.TODO(), BuildOptions{
					Image:   "some/app",
					Builder: builderName,
					AppPath: filepath.Join("testdata", "some-app"),
				}))
				absPath, err := filepath.Abs(filepath.Join("testdata", "some-app"))
				h.AssertNil(t, err)
				h.AssertEq(t, fakeLifecycle.Opts.AppPath, absPath)
			})

			when("appDir is a symlink", func() {
				var (
					appDirName     = "some-app"
					absoluteAppDir string
					tmpDir         string
					err            error
				)

				it.Before(func() {
					tmpDir, err = ioutil.TempDir("", "build-symlink-test")
					h.AssertNil(t, err)

					appDirPath := filepath.Join(tmpDir, appDirName)
					h.AssertNil(t, os.MkdirAll(filepath.Join(tmpDir, appDirName), 0666))

					absoluteAppDir, err = filepath.Abs(appDirPath)
					h.AssertNil(t, err)

					absoluteAppDir, err = filepath.EvalSymlinks(appDirPath)
					h.AssertNil(t, err)
				})

				it.After(func() {
					os.RemoveAll(tmpDir)
				})

				it("resolves relative symbolic links", func() {
					relLink := filepath.Join(tmpDir, "some-app.link")
					h.AssertNil(t, os.Symlink(filepath.Join(".", appDirName), relLink))

					h.AssertNil(t, subject.Build(context.TODO(), BuildOptions{
						Image:   "some/app",
						Builder: builderName,
						AppPath: relLink,
					}))

					h.AssertEq(t, fakeLifecycle.Opts.AppPath, absoluteAppDir)
				})

				it("resolves absolute symbolic links", func() {
					relLink := filepath.Join(tmpDir, "some-app.link")
					h.AssertNil(t, os.Symlink(absoluteAppDir, relLink))

					h.AssertNil(t, subject.Build(context.TODO(), BuildOptions{
						Image:   "some/app",
						Builder: builderName,
						AppPath: relLink,
					}))

					h.AssertEq(t, fakeLifecycle.Opts.AppPath, absoluteAppDir)
				})

				it("resolves symbolic links recursively", func() {
					linkRef1 := absoluteAppDir
					absoluteLink1 := filepath.Join(tmpDir, "some-app-abs-1.link")

					linkRef2 := "some-app-abs-1.link"
					symbolicLink := filepath.Join(tmpDir, "some-app-rel-2.link")

					h.AssertNil(t, os.Symlink(linkRef1, absoluteLink1))
					h.AssertNil(t, os.Symlink(linkRef2, symbolicLink))

					h.AssertNil(t, subject.Build(context.TODO(), BuildOptions{
						Image:   "some/app",
						Builder: builderName,
						AppPath: symbolicLink,
					}))

					h.AssertEq(t, fakeLifecycle.Opts.AppPath, absoluteAppDir)
				})
			})
		})

		when("Builder option", func() {
			it("builder is required", func() {
				h.AssertError(t, subject.Build(context.TODO(), BuildOptions{
					Image: "some/app",
				}),
					"invalid builder ''",
				)
			})

			when("the builder name is provided", func() {
				var (
					customBuilderImage *fakes.Image
					fakeRunImage       *fakes.Image
				)

				it.Before(func() {
					customBuilderImage = ifakes.NewFakeBuilderImage(t,
						tmpDir,
						builderName,
						"some.stack.id",
						"1234",
						"5678",
						builder.Metadata{
							Stack: builder.StackMetadata{
								RunImage: builder.RunImageMetadata{
									Image: "some/run",
								},
							},
							Lifecycle: builder.LifecycleMetadata{
								LifecycleInfo: builder.LifecycleInfo{
									Version: &builder.Version{
										Version: *semver.MustParse("0.3.0"),
									},
								},
								API: builder.LifecycleAPI{
									BuildpackVersion: api.MustParse("0.3"),
									PlatformVersion:  api.MustParse("0.2"),
								},
							},
						},
						nil,
						nil,
					)

					fakeImageFetcher.LocalImages[customBuilderImage.Name()] = customBuilderImage

					fakeRunImage = fakes.NewImage("some/run", "", nil)
					h.AssertNil(t, fakeRunImage.SetLabel("io.buildpacks.stack.id", "some.stack.id"))
					fakeImageFetcher.LocalImages[fakeRunImage.Name()] = fakeRunImage
				})

				it.After(func() {
					customBuilderImage.Cleanup()
					fakeRunImage.Cleanup()
				})

				it("it uses the provided builder", func() {
					h.AssertNil(t, subject.Build(context.TODO(), BuildOptions{
						Image:   "some/app",
						Builder: builderName,
					}))
					h.AssertEq(t, fakeLifecycle.Opts.Builder.Name(), customBuilderImage.Name())
				})
			})
		})

		when("RunImage option", func() {
			var (
				fakeRunImage *fakes.Image
			)

			it.Before(func() {
				fakeRunImage = fakes.NewImage("custom/run", "", nil)
				h.AssertNil(t, fakeRunImage.SetLabel("io.buildpacks.stack.id", defaultBuilderStackID))
				h.AssertNil(t, fakeRunImage.SetLabel("io.buildpacks.stack.mixins", `["mixinA", "mixinX", "run:mixinZ"]`))
				fakeImageFetcher.LocalImages[fakeRunImage.Name()] = fakeRunImage
			})

			it.After(func() {
				fakeRunImage.Cleanup()
			})

			when("run image stack matches the builder stack", func() {
				it("uses the provided image", func() {
					h.AssertNil(t, subject.Build(context.TODO(), BuildOptions{
						Image:    "some/app",
						Builder:  builderName,
						RunImage: "custom/run",
					}))
					h.AssertEq(t, fakeLifecycle.Opts.RunImage, "custom/run")
				})
			})

			when("run image stack does not match the builder stack", func() {
				it.Before(func() {
					h.AssertNil(t, fakeRunImage.SetLabel("io.buildpacks.stack.id", "other.stack"))
				})

				it("errors", func() {
					h.AssertError(t, subject.Build(context.TODO(), BuildOptions{
						Image:    "some/app",
						Builder:  builderName,
						RunImage: "custom/run",
					}),
						"invalid run-image 'custom/run': run-image stack id 'other.stack' does not match builder stack 'some.stack.id'",
					)
				})
			})

			when("run image is not supplied", func() {
				when("there are no locally configured mirrors", func() {
					it("chooses the best mirror from the builder", func() {
						h.AssertNil(t, subject.Build(context.TODO(), BuildOptions{
							Image:   "some/app",
							Builder: builderName,
						}))
						h.AssertEq(t, fakeLifecycle.Opts.RunImage, "default/run")
					})

					it("chooses the best mirror from the builder", func() {
						h.AssertNil(t, subject.Build(context.TODO(), BuildOptions{
							Image:   "registry1.example.com/some/app",
							Builder: builderName,
						}))
						h.AssertEq(t, fakeLifecycle.Opts.RunImage, "registry1.example.com/run/mirror")
					})

					it("chooses the best mirror from the builder", func() {
						h.AssertNil(t, subject.Build(context.TODO(), BuildOptions{
							Image:   "registry2.example.com/some/app",
							Builder: builderName,
						}))
						h.AssertEq(t, fakeLifecycle.Opts.RunImage, "registry2.example.com/run/mirror")
					})
				})

				when("there are locally configured mirrors", func() {
					var (
						fakeLocalMirror  *fakes.Image
						fakeLocalMirror1 *fakes.Image
					)

					it.Before(func() {
						fakeLocalMirror = fakes.NewImage("local/mirror", "", nil)
						h.AssertNil(t, fakeLocalMirror.SetLabel("io.buildpacks.stack.id", defaultBuilderStackID))
						h.AssertNil(t, fakeLocalMirror.SetLabel("io.buildpacks.stack.mixins", `["mixinA", "mixinX", "run:mixinZ"]`))

						fakeImageFetcher.LocalImages[fakeLocalMirror.Name()] = fakeLocalMirror

						fakeLocalMirror1 = fakes.NewImage("registry1.example.com/local/mirror", "", nil)
						h.AssertNil(t, fakeLocalMirror1.SetLabel("io.buildpacks.stack.id", defaultBuilderStackID))
						h.AssertNil(t, fakeLocalMirror1.SetLabel("io.buildpacks.stack.mixins", `["mixinA", "mixinX", "run:mixinZ"]`))

						fakeImageFetcher.LocalImages[fakeLocalMirror1.Name()] = fakeLocalMirror1
					})

					it.After(func() {
						fakeLocalMirror.Cleanup()
						fakeLocalMirror1.Cleanup()
					})

					it("prefers user provided mirrors", func() {
						h.AssertNil(t, subject.Build(context.TODO(), BuildOptions{
							Image:   "some/app",
							Builder: builderName,
							AdditionalMirrors: map[string][]string{
								"default/run": {"local/mirror", "registry1.example.com/local/mirror"},
							},
						}))
						h.AssertEq(t, fakeLifecycle.Opts.RunImage, "local/mirror")
					})

					it("choose the correct user provided mirror for the registry", func() {
						h.AssertNil(t, subject.Build(context.TODO(), BuildOptions{
							Image:   "registry1.example.com/some/app",
							Builder: builderName,
							AdditionalMirrors: map[string][]string{
								"default/run": {"local/mirror", "registry1.example.com/local/mirror"},
							},
						}))
						h.AssertEq(t, fakeLifecycle.Opts.RunImage, "registry1.example.com/local/mirror")
					})

					when("there is no user provided mirror for the registry", func() {
						it("chooses from builder mirrors", func() {
							h.AssertNil(t, subject.Build(context.TODO(), BuildOptions{
								Image:   "registry2.example.com/some/app",
								Builder: builderName,
								AdditionalMirrors: map[string][]string{
									"default/run": {"local/mirror", "registry1.example.com/local/mirror"},
								},
							}))
							h.AssertEq(t, fakeLifecycle.Opts.RunImage, "registry2.example.com/run/mirror")
						})
					})
				})
			})
		})

		when("ClearCache option", func() {
			it("passes it through to lifecycle", func() {
				h.AssertNil(t, subject.Build(context.TODO(), BuildOptions{
					Image:      "some/app",
					Builder:    builderName,
					ClearCache: true,
				}))
				h.AssertEq(t, fakeLifecycle.Opts.ClearCache, true)
			})

			it("defaults to false", func() {
				h.AssertNil(t, subject.Build(context.TODO(), BuildOptions{
					Image:   "some/app",
					Builder: builderName,
				}))
				h.AssertEq(t, fakeLifecycle.Opts.ClearCache, false)
			})
		})

		when("Buildpacks option", func() {
			assertOrderEquals := func(content string) {
				t.Helper()

				orderLayer, err := defaultBuilderImage.FindLayerWithPath("/cnb/order.toml")
				h.AssertNil(t, err)
				h.AssertOnTarEntry(t, orderLayer, "/cnb/order.toml", h.ContentEquals(content))
			}

			it("builder order is overwritten", func() {
				additionalBP := createBuildpackTar(t, tmpDir, dist.BuildpackDescriptor{
					API: api.MustParse("0.3"),
					Info: dist.BuildpackInfo{
						ID:      "buildpack.add.1.id",
						Version: "buildpack.add.1.version",
					},
					Stacks: []dist.Stack{{ID: defaultBuilderStackID}},
					Order:  nil,
				})

				h.AssertNil(t, subject.Build(context.TODO(), BuildOptions{
					Image:      "some/app",
					Builder:    builderName,
					ClearCache: true,
					Buildpacks: []string{additionalBP},
				}))
				h.AssertEq(t, fakeLifecycle.Opts.Builder.Name(), defaultBuilderImage.Name())

				assertOrderEquals(`[[order]]

  [[order.group]]
    id = "buildpack.add.1.id"
    version = "buildpack.add.1.version"
`)
			})

			when("id - no version is provided", func() {
				it("resolves version", func() {
					h.AssertNil(t, subject.Build(context.TODO(), BuildOptions{
						Image:      "some/app",
						Builder:    builderName,
						ClearCache: true,
						Buildpacks: []string{"buildpack.1.id"},
					}))
					h.AssertEq(t, fakeLifecycle.Opts.Builder.Name(), defaultBuilderImage.Name())

					assertOrderEquals(`[[order]]

  [[order.group]]
    id = "buildpack.1.id"
    version = "buildpack.1.version"
`)
				})
			})

			when("from=builder:id@version", func() {
				it("builder order is prepended", func() {
					h.AssertNil(t, subject.Build(context.TODO(), BuildOptions{
						Image:      "some/app",
						Builder:    builderName,
						ClearCache: true,
						Buildpacks: []string{
							"from=builder:buildpack.1.id@buildpack.1.version",
						},
					}))
					h.AssertEq(t, fakeLifecycle.Opts.Builder.Name(), defaultBuilderImage.Name())

					assertOrderEquals(`[[order]]

  [[order.group]]
    id = "buildpack.1.id"
    version = "buildpack.1.version"
`)
				})
			})

			when("from=builder is set first", func() {
				it("builder order is prepended", func() {
					additionalBP1 := createBuildpackTar(t, tmpDir, dist.BuildpackDescriptor{
						API: api.MustParse("0.3"),
						Info: dist.BuildpackInfo{
							ID:      "buildpack.add.1.id",
							Version: "buildpack.add.1.version",
						},
						Stacks: []dist.Stack{{ID: defaultBuilderStackID}},
						Order:  nil,
					})

					additionalBP2 := createBuildpackTar(t, tmpDir, dist.BuildpackDescriptor{
						API: api.MustParse("0.3"),
						Info: dist.BuildpackInfo{
							ID:      "buildpack.add.2.id",
							Version: "buildpack.add.2.version",
						},
						Stacks: []dist.Stack{{ID: defaultBuilderStackID}},
						Order:  nil,
					})

					h.AssertNil(t, subject.Build(context.TODO(), BuildOptions{
						Image:      "some/app",
						Builder:    builderName,
						ClearCache: true,
						Buildpacks: []string{
							"from=builder",
							additionalBP1,
							additionalBP2,
						},
					}))

					assertOrderEquals(`[[order]]

  [[order.group]]
    id = "buildpack.1.id"
    version = "buildpack.1.version"

  [[order.group]]
    id = "buildpack.add.1.id"
    version = "buildpack.add.1.version"

  [[order.group]]
    id = "buildpack.add.2.id"
    version = "buildpack.add.2.version"

[[order]]

  [[order.group]]
    id = "buildpack.2.id"
    version = "buildpack.2.version"

  [[order.group]]
    id = "buildpack.add.1.id"
    version = "buildpack.add.1.version"

  [[order.group]]
    id = "buildpack.add.2.id"
    version = "buildpack.add.2.version"
`)
				})
			})

			when("from=builder is set in middle", func() {
				it("builder order is appended", func() {
					additionalBP1 := createBuildpackTar(t, tmpDir, dist.BuildpackDescriptor{
						API: api.MustParse("0.3"),
						Info: dist.BuildpackInfo{
							ID:      "buildpack.add.1.id",
							Version: "buildpack.add.1.version",
						},
						Stacks: []dist.Stack{{ID: defaultBuilderStackID}},
						Order:  nil,
					})

					additionalBP2 := createBuildpackTar(t, tmpDir, dist.BuildpackDescriptor{
						API: api.MustParse("0.3"),
						Info: dist.BuildpackInfo{
							ID:      "buildpack.add.2.id",
							Version: "buildpack.add.2.version",
						},
						Stacks: []dist.Stack{{ID: defaultBuilderStackID}},
						Order:  nil,
					})

					h.AssertNil(t, subject.Build(context.TODO(), BuildOptions{
						Image:      "some/app",
						Builder:    builderName,
						ClearCache: true,
						Buildpacks: []string{
							additionalBP1,
							"from=builder",
							additionalBP2,
						},
					}))
					h.AssertEq(t, fakeLifecycle.Opts.Builder.Name(), defaultBuilderImage.Name())

					assertOrderEquals(`[[order]]

  [[order.group]]
    id = "buildpack.add.1.id"
    version = "buildpack.add.1.version"

  [[order.group]]
    id = "buildpack.1.id"
    version = "buildpack.1.version"

  [[order.group]]
    id = "buildpack.add.2.id"
    version = "buildpack.add.2.version"

[[order]]

  [[order.group]]
    id = "buildpack.add.1.id"
    version = "buildpack.add.1.version"

  [[order.group]]
    id = "buildpack.2.id"
    version = "buildpack.2.version"

  [[order.group]]
    id = "buildpack.add.2.id"
    version = "buildpack.add.2.version"
`)
				})
			})

			when("from=builder is set last", func() {
				it("builder order is appended", func() {
					additionalBP1 := createBuildpackTar(t, tmpDir, dist.BuildpackDescriptor{
						API: api.MustParse("0.3"),
						Info: dist.BuildpackInfo{
							ID:      "buildpack.add.1.id",
							Version: "buildpack.add.1.version",
						},
						Stacks: []dist.Stack{{ID: defaultBuilderStackID}},
						Order:  nil,
					})

					additionalBP2 := createBuildpackTar(t, tmpDir, dist.BuildpackDescriptor{
						API: api.MustParse("0.3"),
						Info: dist.BuildpackInfo{
							ID:      "buildpack.add.2.id",
							Version: "buildpack.add.2.version",
						},
						Stacks: []dist.Stack{{ID: defaultBuilderStackID}},
						Order:  nil,
					})

					h.AssertNil(t, subject.Build(context.TODO(), BuildOptions{
						Image:      "some/app",
						Builder:    builderName,
						ClearCache: true,
						Buildpacks: []string{
							additionalBP1,
							additionalBP2,
							"from=builder",
						},
					}))
					h.AssertEq(t, fakeLifecycle.Opts.Builder.Name(), defaultBuilderImage.Name())

					assertOrderEquals(`[[order]]

  [[order.group]]
    id = "buildpack.add.1.id"
    version = "buildpack.add.1.version"

  [[order.group]]
    id = "buildpack.add.2.id"
    version = "buildpack.add.2.version"

  [[order.group]]
    id = "buildpack.1.id"
    version = "buildpack.1.version"

[[order]]

  [[order.group]]
    id = "buildpack.add.1.id"
    version = "buildpack.add.1.version"

  [[order.group]]
    id = "buildpack.add.2.id"
    version = "buildpack.add.2.version"

  [[order.group]]
    id = "buildpack.2.id"
    version = "buildpack.2.version"
`)
				})
			})

			when("meta-buildpack is used", func() {
				it("resolves buildpack from builder", func() {
					buildpackTar := createBuildpackTar(t, tmpDir, dist.BuildpackDescriptor{
						API: api.MustParse("0.3"),
						Info: dist.BuildpackInfo{
							ID:      "metabuildpack.id",
							Version: "metabuildpack.version",
						},
						Stacks: nil,
						Order: dist.Order{{
							Group: []dist.BuildpackRef{{
								BuildpackInfo: dist.BuildpackInfo{
									ID:      "buildpack.1.id",
									Version: "buildpack.1.version",
								},
								Optional: false,
							}, {
								BuildpackInfo: dist.BuildpackInfo{
									ID:      "buildpack.2.id",
									Version: "buildpack.2.version",
								},
								Optional: false,
							}},
						}},
					})

					err := subject.Build(context.TODO(), BuildOptions{
						Image:      "some/app",
						Builder:    builderName,
						ClearCache: true,
						Buildpacks: []string{buildpackTar},
					})

					h.AssertNil(t, err)
				})
			})

			when("buildpackage image is used", func() {
				var fakePackage *fakes.Image

				it.Before(func() {
					metaBuildpackTar := createBuildpackTar(t, tmpDir, dist.BuildpackDescriptor{
						API: api.MustParse("0.3"),
						Info: dist.BuildpackInfo{
							ID:      "meta.buildpack.id",
							Version: "meta.buildpack.version",
						},
						Stacks: nil,
						Order: dist.Order{{
							Group: []dist.BuildpackRef{{
								BuildpackInfo: dist.BuildpackInfo{
									ID:      "child.buildpack.id",
									Version: "child.buildpack.version",
								},
								Optional: false,
							}},
						}},
					})

					childBuildpackTar := createBuildpackTar(t, tmpDir, dist.BuildpackDescriptor{
						API: api.MustParse("0.3"),
						Info: dist.BuildpackInfo{
							ID:      "child.buildpack.id",
							Version: "child.buildpack.version",
						},
						Stacks: []dist.Stack{
							{ID: defaultBuilderStackID},
						},
					})

					bpLayers := dist.BuildpackLayers{
						"meta.buildpack.id": {
							"meta.buildpack.version": {
								API: api.MustParse("0.3"),
								Order: dist.Order{{
									Group: []dist.BuildpackRef{{
										BuildpackInfo: dist.BuildpackInfo{
											ID:      "child.buildpack.id",
											Version: "child.buildpack.version",
										},
										Optional: false,
									}},
								}},
								LayerDiffID: diffIDForFile(t, metaBuildpackTar),
							},
						},
						"child.buildpack.id": {
							"child.buildpack.version": {
								API: api.MustParse("0.3"),
								Stacks: []dist.Stack{
									{ID: defaultBuilderStackID},
								},
								LayerDiffID: diffIDForFile(t, childBuildpackTar),
							},
						},
					}

					md := buildpackage.Metadata{
						BuildpackInfo: dist.BuildpackInfo{
							ID:      "meta.buildpack.id",
							Version: "meta.buildpack.version",
						},
						Stacks: []dist.Stack{
							{ID: defaultBuilderStackID},
						},
					}

					fakePackage = fakes.NewImage("example.com/some/package", "", nil)
					h.AssertNil(t, dist.SetLabel(fakePackage, "io.buildpacks.buildpack.layers", bpLayers))
					h.AssertNil(t, dist.SetLabel(fakePackage, "io.buildpacks.buildpackage.metadata", md))

					h.AssertNil(t, fakePackage.AddLayer(metaBuildpackTar))
					h.AssertNil(t, fakePackage.AddLayer(childBuildpackTar))

					fakeImageFetcher.LocalImages[fakePackage.Name()] = fakePackage
				})

				it("all buildpacks are added to ephemeral builder", func() {
					err := subject.Build(context.TODO(), BuildOptions{
						Image:      "some/app",
						Builder:    builderName,
						ClearCache: true,
						Buildpacks: []string{
							"example.com/some/package",
						},
					})

					h.AssertNil(t, err)
					h.AssertEq(t, fakeLifecycle.Opts.Builder.Name(), defaultBuilderImage.Name())
					bldr, err := builder.FromImage(defaultBuilderImage)
					h.AssertNil(t, err)
					h.AssertEq(t, bldr.Order(), dist.Order{
						{Group: []dist.BuildpackRef{
							{BuildpackInfo: dist.BuildpackInfo{ID: "meta.buildpack.id", Version: "meta.buildpack.version"}},
						}},
						// Child buildpacks should not be added to order
					})
					h.AssertEq(t, bldr.Buildpacks(), []dist.BuildpackInfo{
						{
							ID:      "buildpack.1.id",
							Version: "buildpack.1.version",
						},
						{
							ID:      "buildpack.2.id",
							Version: "buildpack.2.version",
						},
						{
							ID:      "meta.buildpack.id",
							Version: "meta.buildpack.version",
						},
						{
							ID:      "child.buildpack.id",
							Version: "child.buildpack.version",
						},
					})
				})

				it("fails when no metadata label on package", func() {
					h.AssertNil(t, fakePackage.SetLabel("io.buildpacks.buildpackage.metadata", ""))

					err := subject.Build(context.TODO(), BuildOptions{
						Image:      "some/app",
						Builder:    builderName,
						ClearCache: true,
						Buildpacks: []string{
							"example.com/some/package",
						},
					})

					h.AssertError(t, err, "could not find label 'io.buildpacks.buildpackage.metadata' on image 'example.com/some/package'")
				})

				it("fails when no bp layers label is on package", func() {
					h.AssertNil(t, fakePackage.SetLabel("io.buildpacks.buildpack.layers", ""))

					err := subject.Build(context.TODO(), BuildOptions{
						Image:      "some/app",
						Builder:    builderName,
						ClearCache: true,
						Buildpacks: []string{
							"example.com/some/package",
						},
					})

					h.AssertError(t, err, "could not find label 'io.buildpacks.buildpack.layers' on image 'example.com/some/package'")
				})
			})

			it("ensures buildpacks exist on builder", func() {
				h.AssertError(t, subject.Build(context.TODO(), BuildOptions{
					Image:      "some/app",
					Builder:    builderName,
					ClearCache: true,
					Buildpacks: []string{"missing.bp@version"},
				}),
					"invalid buildpack string 'missing.bp@version'",
				)
			})

			when("buildpacks include URIs", func() {
				var buildpackTgz string

				it.Before(func() {
					buildpackTgz = h.CreateTGZ(t, filepath.Join("testdata", "buildpack2"), "./", 0755)
				})

				it.After(func() {
					h.AssertNil(t, os.Remove(buildpackTgz))
				})

				when("is windows", func() {
					it.Before(func() {
						h.SkipIf(t, runtime.GOOS != "windows", "Skipped on non-windows")
					})

					it("disallows directory-based buildpacks", func() {
						err := subject.Build(context.TODO(), BuildOptions{
							Image:      "some/app",
							Builder:    builderName,
							ClearCache: true,
							Buildpacks: []string{
								"buildpack.1.id@buildpack.1.version",
								filepath.Join("testdata", "buildpack"),
							},
						})

						h.AssertError(t, err, fmt.Sprintf("buildpack '%s': directory-based buildpacks are not currently supported on Windows", filepath.Join("testdata", "buildpack")))
					})

					it("buildpacks are added to ephemeral builder", func() {
						err := subject.Build(context.TODO(), BuildOptions{
							Image:      "some/app",
							Builder:    builderName,
							ClearCache: true,
							Buildpacks: []string{
								"buildpack.1.id@buildpack.1.version",
								buildpackTgz,
							},
						})

						h.AssertNil(t, err)
						h.AssertEq(t, fakeLifecycle.Opts.Builder.Name(), defaultBuilderImage.Name())
						bldr, err := builder.FromImage(defaultBuilderImage)
						h.AssertNil(t, err)
						h.AssertEq(t, bldr.Order(), dist.Order{
							{Group: []dist.BuildpackRef{
								{BuildpackInfo: dist.BuildpackInfo{ID: "buildpack.1.id", Version: "buildpack.1.version"}},
								{BuildpackInfo: dist.BuildpackInfo{ID: "some-other-buildpack-id", Version: "some-other-buildpack-version"}},
							}},
						})
						h.AssertEq(t, bldr.Buildpacks(), []dist.BuildpackInfo{
							{
								ID:      "buildpack.1.id",
								Version: "buildpack.1.version",
							},
							{
								ID:      "buildpack.2.id",
								Version: "buildpack.2.version",
							},
							{
								ID:      "some-other-buildpack-id",
								Version: "some-other-buildpack-version",
							},
						})
					})
				})

				when("is posix", func() {
					it.Before(func() {
						h.SkipIf(t, runtime.GOOS == "windows", "Skipped on windows")
					})

					it("buildpacks are added to ephemeral builder", func() {
						err := subject.Build(context.TODO(), BuildOptions{
							Image:      "some/app",
							Builder:    builderName,
							ClearCache: true,
							Buildpacks: []string{
								"buildpack.1.id@buildpack.1.version",
								"buildpack.2.id@buildpack.2.version",
								filepath.Join("testdata", "buildpack"),
								buildpackTgz,
							},
						})

						h.AssertNil(t, err)
						h.AssertEq(t, fakeLifecycle.Opts.Builder.Name(), defaultBuilderImage.Name())
						bldr, err := builder.FromImage(defaultBuilderImage)
						h.AssertNil(t, err)
						buildpack1Info := dist.BuildpackInfo{ID: "buildpack.1.id", Version: "buildpack.1.version"}
						buildpack2Info := dist.BuildpackInfo{ID: "buildpack.2.id", Version: "buildpack.2.version"}
						dirBuildpackInfo := dist.BuildpackInfo{ID: "bp.one", Version: "1.2.3"}
						tgzBuildpackInfo := dist.BuildpackInfo{ID: "some-other-buildpack-id", Version: "some-other-buildpack-version"}
						h.AssertEq(t, bldr.Order(), dist.Order{
							{Group: []dist.BuildpackRef{
								{BuildpackInfo: buildpack1Info},
								{BuildpackInfo: buildpack2Info},
								{BuildpackInfo: dirBuildpackInfo},
								{BuildpackInfo: tgzBuildpackInfo},
							}},
						})
						h.AssertEq(t, bldr.Buildpacks(), []dist.BuildpackInfo{
							buildpack1Info,
							buildpack2Info,
							dirBuildpackInfo,
							tgzBuildpackInfo,
						})
					})
				})

				when("uri is a http url", func() {
					var server *ghttp.Server

					it.Before(func() {
						server = ghttp.NewServer()
						server.AppendHandlers(func(w http.ResponseWriter, r *http.Request) {
							http.ServeFile(w, r, buildpackTgz)
						})
					})

					it.After(func() {
						server.Close()
					})

					it("adds the buildpack", func() {
						err := subject.Build(context.TODO(), BuildOptions{
							Image:      "some/app",
							Builder:    builderName,
							ClearCache: true,
							Buildpacks: []string{
								"buildpack.1.id@buildpack.1.version",
								"buildpack.2.id@buildpack.2.version",
								server.URL(),
							},
						})

						h.AssertNil(t, err)
						h.AssertEq(t, fakeLifecycle.Opts.Builder.Name(), defaultBuilderImage.Name())
						bldr, err := builder.FromImage(defaultBuilderImage)
						h.AssertNil(t, err)
						h.AssertEq(t, bldr.Order(), dist.Order{
							{Group: []dist.BuildpackRef{
								{BuildpackInfo: dist.BuildpackInfo{ID: "buildpack.1.id", Version: "buildpack.1.version"}},
								{BuildpackInfo: dist.BuildpackInfo{ID: "buildpack.2.id", Version: "buildpack.2.version"}},
								{BuildpackInfo: dist.BuildpackInfo{ID: "some-other-buildpack-id", Version: "some-other-buildpack-version"}},
							}},
						})
						h.AssertEq(t, bldr.Buildpacks(), []dist.BuildpackInfo{
							{ID: "buildpack.1.id", Version: "buildpack.1.version"},
							{ID: "buildpack.2.id", Version: "buildpack.2.version"},
							{ID: "some-other-buildpack-id", Version: "some-other-buildpack-version"},
						})
					})
				})

				when("added buildpack's mixins are not satisfied", func() {
					it.Before(func() {
						h.AssertNil(t, defaultBuilderImage.SetLabel("io.buildpacks.stack.mixins", `["mixinX", "build:mixinY"]`))
						h.AssertNil(t, fakeDefaultRunImage.SetLabel("io.buildpacks.stack.mixins", `["mixinX", "run:mixinZ"]`))
					})

					it("returns an error", func() {
						err := subject.Build(context.TODO(), BuildOptions{
							Image:   "some/app",
							Builder: builderName,
							Buildpacks: []string{
								buildpackTgz, // requires mixinA, build:mixinB, run:mixinC
							},
						})

						h.AssertError(t, err, "validating stack mixins: buildpack 'some-other-buildpack-id@some-other-buildpack-version' requires missing mixin(s): build:mixinB, mixinA, run:mixinC")
					})
				})
			})
		})

		when("Env option", func() {
			it("should set the env on the ephemeral builder", func() {
				h.AssertNil(t, subject.Build(context.TODO(), BuildOptions{
					Image:   "some/app",
					Builder: builderName,
					Env: map[string]string{
						"key1": "value1",
						"key2": "value2",
					},
				}))
				layerTar, err := defaultBuilderImage.FindLayerWithPath("/platform/env/key1")
				h.AssertNil(t, err)
				assertTarFileContents(t, layerTar, "/platform/env/key1", `value1`)
				assertTarFileContents(t, layerTar, "/platform/env/key2", `value2`)
			})
		})

		when("Publish option", func() {
			when("true", func() {
				var remoteRunImage *fakes.Image

				it.Before(func() {
					remoteRunImage = fakes.NewImage("default/run", "", nil)
					h.AssertNil(t, remoteRunImage.SetLabel("io.buildpacks.stack.id", defaultBuilderStackID))
					h.AssertNil(t, remoteRunImage.SetLabel("io.buildpacks.stack.mixins", `["mixinA", "mixinX", "run:mixinZ"]`))
					fakeImageFetcher.RemoteImages[remoteRunImage.Name()] = remoteRunImage
				})

				it.After(func() {
					remoteRunImage.Cleanup()
				})

				it("uses a remote run image", func() {
					h.AssertNil(t, subject.Build(context.TODO(), BuildOptions{
						Image:   "some/app",
						Builder: builderName,
						Publish: true,
					}))
					h.AssertEq(t, fakeLifecycle.Opts.Publish, true)

					args := fakeImageFetcher.FetchCalls["default/run"]
					h.AssertEq(t, args.Daemon, false)

					args = fakeImageFetcher.FetchCalls[builderName]
					h.AssertEq(t, args.Daemon, true)
				})

				when("false", func() {
					it("uses a local run image", func() {
						h.AssertNil(t, subject.Build(context.TODO(), BuildOptions{
							Image:   "some/app",
							Builder: builderName,
							Publish: false,
						}))
						h.AssertEq(t, fakeLifecycle.Opts.Publish, false)

						args := fakeImageFetcher.FetchCalls["default/run"]
						h.AssertEq(t, args.Daemon, true)
						h.AssertEq(t, args.Pull, true)

						args = fakeImageFetcher.FetchCalls[builderName]
						h.AssertEq(t, args.Daemon, true)
						h.AssertEq(t, args.Pull, true)
					})
				})
			})

			when("NoPull option", func() {
				when("true", func() {
					it("uses the local builder and run images without updating", func() {
						h.AssertNil(t, subject.Build(context.TODO(), BuildOptions{
							Image:   "some/app",
							Builder: builderName,
							NoPull:  true,
						}))

						args := fakeImageFetcher.FetchCalls["default/run"]
						h.AssertEq(t, args.Daemon, true)
						h.AssertEq(t, args.Pull, false)

						args = fakeImageFetcher.FetchCalls[builderName]
						h.AssertEq(t, args.Daemon, true)
						h.AssertEq(t, args.Pull, false)
					})
				})

				when("false", func() {
					it("uses pulls the builder and run image before using them", func() {
						h.AssertNil(t, subject.Build(context.TODO(), BuildOptions{
							Image:   "some/app",
							Builder: builderName,
							NoPull:  false,
						}))

						args := fakeImageFetcher.FetchCalls["default/run"]
						h.AssertEq(t, args.Daemon, true)
						h.AssertEq(t, args.Pull, true)

						args = fakeImageFetcher.FetchCalls[builderName]
						h.AssertEq(t, args.Daemon, true)
						h.AssertEq(t, args.Pull, true)
					})
				})
			})

			when("ProxyConfig option", func() {
				when("ProxyConfig is nil", func() {
					it.Before(func() {
						h.AssertNil(t, os.Setenv("http_proxy", "other-http-proxy"))
						h.AssertNil(t, os.Setenv("https_proxy", "other-https-proxy"))
						h.AssertNil(t, os.Setenv("no_proxy", "other-no-proxy"))
					})

					when("*_PROXY env vars are set", func() {
						it.Before(func() {
							h.AssertNil(t, os.Setenv("HTTP_PROXY", "some-http-proxy"))
							h.AssertNil(t, os.Setenv("HTTPS_PROXY", "some-https-proxy"))
							h.AssertNil(t, os.Setenv("NO_PROXY", "some-no-proxy"))
						})

						it.After(func() {
							h.AssertNil(t, os.Unsetenv("HTTP_PROXY"))
							h.AssertNil(t, os.Unsetenv("HTTPS_PROXY"))
							h.AssertNil(t, os.Unsetenv("NO_PROXY"))
						})

						it("defaults to the *_PROXY environment variables", func() {
							h.AssertNil(t, subject.Build(context.TODO(), BuildOptions{
								Image:   "some/app",
								Builder: builderName,
							}))
							h.AssertEq(t, fakeLifecycle.Opts.HTTPProxy, "some-http-proxy")
							h.AssertEq(t, fakeLifecycle.Opts.HTTPSProxy, "some-https-proxy")
							h.AssertEq(t, fakeLifecycle.Opts.NoProxy, "some-no-proxy")
						})
					})

					it("falls back to the *_proxy environment variables", func() {
						h.AssertNil(t, subject.Build(context.TODO(), BuildOptions{
							Image:   "some/app",
							Builder: builderName,
						}))
						h.AssertEq(t, fakeLifecycle.Opts.HTTPProxy, "other-http-proxy")
						h.AssertEq(t, fakeLifecycle.Opts.HTTPSProxy, "other-https-proxy")
						h.AssertEq(t, fakeLifecycle.Opts.NoProxy, "other-no-proxy")
					})
				}, spec.Sequential())

				when("ProxyConfig is not nil", func() {
					it("passes the values through", func() {
						h.AssertNil(t, subject.Build(context.TODO(), BuildOptions{
							Image:   "some/app",
							Builder: builderName,
							ProxyConfig: &ProxyConfig{
								HTTPProxy:  "custom-http-proxy",
								HTTPSProxy: "custom-https-proxy",
								NoProxy:    "custom-no-proxy",
							},
						}))
						h.AssertEq(t, fakeLifecycle.Opts.HTTPProxy, "custom-http-proxy")
						h.AssertEq(t, fakeLifecycle.Opts.HTTPSProxy, "custom-https-proxy")
						h.AssertEq(t, fakeLifecycle.Opts.NoProxy, "custom-no-proxy")
					})
				})
			})

			when("Network option", func() {
				it("passes the value through", func() {
					h.AssertNil(t, subject.Build(context.TODO(), BuildOptions{
						Image:   "some/app",
						Builder: builderName,
						ContainerConfig: ContainerConfig{
							Network: "some-network",
						},
					}))
					h.AssertEq(t, fakeLifecycle.Opts.Network, "some-network")
				})
			})
		})

		when("Lifecycle option", func() {
			when("Platform API", func() {
				when("lifecycle platform API is compatible", func() {
					it("should succeed", func() {
						err := subject.Build(context.TODO(), BuildOptions{
							Image:   "some/app",
							Builder: builderName,
						})

						h.AssertNil(t, err)
					})
				})

				when("lifecycle platform API is not compatible", func() {
					var incompatibleBuilderImage *fakes.Image
					it.Before(func() {
						incompatibleBuilderImage = ifakes.NewFakeBuilderImage(t,
							tmpDir,
							"incompatible-"+builderName,
							defaultBuilderStackID,
							"1234",
							"5678",
							builder.Metadata{
								Stack: builder.StackMetadata{
									RunImage: builder.RunImageMetadata{
										Image: "default/run",
										Mirrors: []string{
											"registry1.example.com/run/mirror",
											"registry2.example.com/run/mirror",
										},
									},
								},
								Lifecycle: builder.LifecycleMetadata{
									LifecycleInfo: builder.LifecycleInfo{
										Version: &builder.Version{
											Version: *semver.MustParse("0.3.0"),
										},
									},
									API: builder.LifecycleAPI{
										BuildpackVersion: api.MustParse("0.3"),
										PlatformVersion:  api.MustParse("0.9"),
									},
								},
							},
							nil,
							nil,
						)

						fakeImageFetcher.LocalImages[incompatibleBuilderImage.Name()] = incompatibleBuilderImage
					})

					it.After(func() {
						incompatibleBuilderImage.Cleanup()
					})

					it("should error", func() {
						builderName := incompatibleBuilderImage.Name()

						err := subject.Build(context.TODO(), BuildOptions{
							Image:   "some/app",
							Builder: builderName,
						})

						h.AssertError(t, err, fmt.Sprintf("Builder %s is incompatible with this version of pack", style.Symbol(builderName)))
					})
				})
			})
		})

		when("validating mixins", func() {
			when("stack image mixins disagree", func() {
				it.Before(func() {
					h.AssertNil(t, defaultBuilderImage.SetLabel("io.buildpacks.stack.mixins", `["mixinA"]`))
					h.AssertNil(t, fakeDefaultRunImage.SetLabel("io.buildpacks.stack.mixins", `["mixinB"]`))
				})

				it("returns an error", func() {
					err := subject.Build(context.TODO(), BuildOptions{
						Image:   "some/app",
						Builder: builderName,
					})

					h.AssertError(t, err, "validating stack mixins: 'default/run' missing required mixin(s): mixinA")
				})
			})

			when("builder buildpack mixins are not satisfied", func() {
				it.Before(func() {
					h.AssertNil(t, defaultBuilderImage.SetLabel("io.buildpacks.stack.mixins", ""))
					h.AssertNil(t, fakeDefaultRunImage.SetLabel("io.buildpacks.stack.mixins", ""))
				})

				it("returns an error", func() {
					err := subject.Build(context.TODO(), BuildOptions{
						Image:   "some/app",
						Builder: builderName,
					})

					h.AssertError(t, err, "validating stack mixins: buildpack 'buildpack.1.id@buildpack.1.version' requires missing mixin(s): build:mixinY, mixinX, run:mixinZ")
				})
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

func createBuildpackTar(t *testing.T, tmpDir string, descriptor dist.BuildpackDescriptor) string {
	buildpack, err := ifakes.NewFakeBuildpackBlob(descriptor, 0777)
	h.AssertNil(t, err)

	tempFile, err := ioutil.TempFile(tmpDir, "bp-*.tar")
	h.AssertNil(t, err)
	defer tempFile.Close()

	reader, err := buildpack.Open()
	h.AssertNil(t, err)

	_, err = io.Copy(tempFile, reader)
	h.AssertNil(t, err)

	return tempFile.Name()
}

func diffIDForFile(t *testing.T, path string) string {
	file, err := os.Open(path)
	h.AssertNil(t, err)

	hasher := sha256.New()
	_, err = io.Copy(hasher, file)
	h.AssertNil(t, err)

	return "sha256:" + hex.EncodeToString(hasher.Sum(make([]byte, 0, hasher.Size())))
}
