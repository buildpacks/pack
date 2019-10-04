package pack

import (
	"archive/tar"
	"bytes"
	"context"
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
	"github.com/buildpack/imgutil/fakes"
	"github.com/docker/docker/client"
	"github.com/heroku/color"
	"github.com/onsi/gomega/ghttp"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	"github.com/buildpack/pack/api"
	"github.com/buildpack/pack/blob"
	"github.com/buildpack/pack/build"
	"github.com/buildpack/pack/builder"
	"github.com/buildpack/pack/cmd"
	"github.com/buildpack/pack/dist"
	ifakes "github.com/buildpack/pack/internal/fakes"
	"github.com/buildpack/pack/logging"
	"github.com/buildpack/pack/style"
	h "github.com/buildpack/pack/testhelpers"
)

func TestBuild(t *testing.T) {
	color.Disable(true)
	defer func() { color.Disable(false) }()
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
			builderName,
			defaultBuilderStackID,
			"1234",
			"5678",
			builder.Metadata{
				Buildpacks: []builder.BuildpackMetadata{
					{
						BuildpackInfo: dist.BuildpackInfo{ID: "buildpack.id", Version: "buildpack.version"},
						Latest:        true,
					},
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
						PlatformVersion:  api.MustParse(build.PlatformAPIVersion),
					},
				},
			},
		)

		fakeImageFetcher.LocalImages[defaultBuilderImage.Name()] = defaultBuilderImage

		fakeDefaultRunImage = fakes.NewImage("default/run", "", nil)
		h.AssertNil(t, fakeDefaultRunImage.SetLabel("io.buildpacks.stack.id", defaultBuilderStackID))
		fakeImageFetcher.LocalImages[fakeDefaultRunImage.Name()] = fakeDefaultRunImage

		fakeMirror1 = fakes.NewImage("registry1.example.com/run/mirror", "", nil)
		h.AssertNil(t, fakeMirror1.SetLabel("io.buildpacks.stack.id", defaultBuilderStackID))
		fakeImageFetcher.LocalImages[fakeMirror1.Name()] = fakeMirror1

		fakeMirror2 = fakes.NewImage("registry2.example.com/run/mirror", "", nil)
		h.AssertNil(t, fakeMirror2.SetLabel("io.buildpacks.stack.id", defaultBuilderStackID))
		fakeImageFetcher.LocalImages[fakeMirror2.Name()] = fakeMirror2

		docker, err := client.NewClientWithOpts(client.FromEnv, client.WithVersion("1.38"))
		h.AssertNil(t, err)

		logger = ifakes.NewFakeLogger(&outBuf)

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
								API: builder.LifecycleAPI{
									PlatformVersion: api.MustParse(build.PlatformAPIVersion),
								},
							},
						})

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
				fakeImageFetcher.LocalImages[fakeRunImage.Name()] = fakeRunImage
			})

			it.After(func() {
				fakeRunImage.Cleanup()
			})

			when("run image stack matches the builder stack", func() {
				it.Before(func() {
					h.AssertNil(t, fakeRunImage.SetLabel("io.buildpacks.stack.id", defaultBuilderStackID))
				})

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
			})

			when("run image is not supplied", func() {
				when("there are locally configured mirrors", func() {
					var (
						fakeLocalMirror  *fakes.Image
						fakeLocalMirror1 *fakes.Image
					)

					it.Before(func() {
						fakeLocalMirror = fakes.NewImage("local/mirror", "", nil)
						h.AssertNil(t, fakeLocalMirror.SetLabel("io.buildpacks.stack.id", defaultBuilderStackID))
						fakeImageFetcher.LocalImages[fakeLocalMirror.Name()] = fakeLocalMirror

						fakeLocalMirror1 = fakes.NewImage("registry1.example.com/local/mirror", "", nil)
						h.AssertNil(t, fakeLocalMirror1.SetLabel("io.buildpacks.stack.id", defaultBuilderStackID))
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
			it("builder order is overwritten", func() {
				h.AssertNil(t, subject.Build(context.TODO(), BuildOptions{
					Image:      "some/app",
					Builder:    builderName,
					ClearCache: true,
					Buildpacks: []string{"buildpack.id@buildpack.version"},
				}))
				h.AssertEq(t, fakeLifecycle.Opts.Builder.Name(), defaultBuilderImage.Name())
				bldr, err := builder.GetBuilder(defaultBuilderImage)
				h.AssertNil(t, err)
				h.AssertEq(t, bldr.GetOrder(), dist.Order{
					{Group: []dist.BuildpackRef{{
						BuildpackInfo: dist.BuildpackInfo{
							ID:      "buildpack.id",
							Version: "buildpack.version",
						}},
					}},
				})
			})

			when("no version is provided", func() {
				it("resolves version", func() {
					h.AssertNil(t, subject.Build(context.TODO(), BuildOptions{
						Image:      "some/app",
						Builder:    builderName,
						ClearCache: true,
						Buildpacks: []string{"buildpack.id"},
					}))
					h.AssertEq(t, fakeLifecycle.Opts.Builder.Name(), defaultBuilderImage.Name())

					orderLayer, err := defaultBuilderImage.FindLayerWithPath("/cnb/order.toml")
					h.AssertNil(t, err)

					h.AssertOnTarEntry(t,
						orderLayer,
						"/cnb/order.toml",
						h.ContentEquals(`[[order]]

  [[order.group]]
    id = "buildpack.id"
    version = "buildpack.version"
`))
				})
			})

			when("latest is explicitly provided", func() {
				it("resolves version and prints a warning", func() {
					h.AssertNil(t, subject.Build(context.TODO(), BuildOptions{
						Image:      "some/app",
						Builder:    builderName,
						ClearCache: true,
						Buildpacks: []string{"buildpack.id@latest"},
					}))
					h.AssertEq(t, fakeLifecycle.Opts.Builder.Name(), defaultBuilderImage.Name())
					h.AssertContains(t, outBuf.String(), "Warning: @latest syntax is deprecated, will not work in future releases")

					orderLayer, err := defaultBuilderImage.FindLayerWithPath("/cnb/order.toml")
					h.AssertNil(t, err)

					h.AssertOnTarEntry(t,
						orderLayer,
						"/cnb/order.toml",
						h.ContentEquals(`[[order]]

  [[order.group]]
    id = "buildpack.id"
    version = "buildpack.version"
`))
				})
			})

			it("ensures buildpacks exist on builder", func() {
				h.AssertError(t, subject.Build(context.TODO(), BuildOptions{
					Image:      "some/app",
					Builder:    builderName,
					ClearCache: true,
					Buildpacks: []string{"missing.bp@version"},
				}),
					"no versions of buildpack 'missing.bp' were found on the builder",
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
								"buildid@buildpack.version",
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
								"buildpack.id@buildpack.version",
								buildpackTgz,
							},
						})

						h.AssertNil(t, err)
						h.AssertEq(t, fakeLifecycle.Opts.Builder.Name(), defaultBuilderImage.Name())
						bldr, err := builder.GetBuilder(defaultBuilderImage)
						h.AssertNil(t, err)
						h.AssertEq(t, bldr.GetOrder(), dist.Order{
							{Group: []dist.BuildpackRef{
								{BuildpackInfo: dist.BuildpackInfo{ID: "buildpack.id", Version: "buildpack.version"}},
								{BuildpackInfo: dist.BuildpackInfo{ID: "some-other-buildpack-id", Version: "some-other-buildpack-version"}},
							}},
						})
						h.AssertEq(t, bldr.GetBuildpacks(), []builder.BuildpackMetadata{
							{
								BuildpackInfo: dist.BuildpackInfo{
									ID:      "buildpack.id",
									Version: "buildpack.version",
								},
								Latest: true,
							},
							{
								BuildpackInfo: dist.BuildpackInfo{
									ID:      "some-other-buildpack-id",
									Version: "some-other-buildpack-version",
								},
								Latest: true,
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
								"buildpack.id@buildpack.version",
								filepath.Join("testdata", "buildpack"),
								buildpackTgz,
							},
						})

						h.AssertNil(t, err)
						h.AssertEq(t, fakeLifecycle.Opts.Builder.Name(), defaultBuilderImage.Name())
						bldr, err := builder.GetBuilder(defaultBuilderImage)
						h.AssertNil(t, err)
						buildpackInfo := dist.BuildpackInfo{ID: "buildpack.id", Version: "buildpack.version"}
						dirBuildpackInfo := dist.BuildpackInfo{ID: "bp.one", Version: "1.2.3"}
						tgzBuildpackInfo := dist.BuildpackInfo{ID: "some-other-buildpack-id", Version: "some-other-buildpack-version"}
						h.AssertEq(t, bldr.GetOrder(), dist.Order{
							{Group: []dist.BuildpackRef{
								{BuildpackInfo: buildpackInfo},
								{BuildpackInfo: dirBuildpackInfo},
								{BuildpackInfo: tgzBuildpackInfo},
							}},
						})
						h.AssertEq(t, bldr.GetBuildpacks(), []builder.BuildpackMetadata{
							{BuildpackInfo: buildpackInfo, Latest: true},
							{BuildpackInfo: dirBuildpackInfo, Latest: true},
							{BuildpackInfo: tgzBuildpackInfo, Latest: true},
						})
					})
				})

				when("uri is a http url", func() {
					var server *ghttp.Server

					it.Before(func() {
						h.SkipIf(t, runtime.GOOS == "windows", "Skipped on windows")
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
								"buildpack.id@buildpack.version",
								filepath.Join("testdata", "buildpack"),
								server.URL(),
							},
						})

						h.AssertNil(t, err)
						h.AssertEq(t, fakeLifecycle.Opts.Builder.Name(), defaultBuilderImage.Name())
						bldr, err := builder.GetBuilder(defaultBuilderImage)
						h.AssertNil(t, err)
						h.AssertEq(t, bldr.GetOrder(), dist.Order{
							{Group: []dist.BuildpackRef{
								{BuildpackInfo: dist.BuildpackInfo{ID: "buildpack.id", Version: "buildpack.version"}},
								{BuildpackInfo: dist.BuildpackInfo{ID: "bp.one", Version: "1.2.3"}},
								{BuildpackInfo: dist.BuildpackInfo{ID: "some-other-buildpack-id", Version: "some-other-buildpack-version"}},
							}},
						})
						h.AssertEq(t, bldr.GetBuildpacks(), []builder.BuildpackMetadata{
							{BuildpackInfo: dist.BuildpackInfo{ID: "buildpack.id", Version: "buildpack.version"}, Latest: true},
							{BuildpackInfo: dist.BuildpackInfo{ID: "bp.one", Version: "1.2.3"}, Latest: true},
							{BuildpackInfo: dist.BuildpackInfo{ID: "some-other-buildpack-id", Version: "some-other-buildpack-version"}, Latest: true},
						})
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

						h.AssertError(t,
							err,
							fmt.Sprintf(
								"pack %s (Platform API version %s) is incompatible with builder %s (Platform API version %s)",
								cmd.Version,
								build.PlatformAPIVersion,
								style.Symbol(builderName),
								"0.9",
							))
					})
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
