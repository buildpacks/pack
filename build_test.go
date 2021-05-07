package pack

import (
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
	"strings"
	"testing"
	"time"

	"github.com/Masterminds/semver"
	"github.com/buildpacks/imgutil"
	"github.com/buildpacks/imgutil/fakes"
	"github.com/buildpacks/imgutil/local"
	"github.com/buildpacks/imgutil/remote"
	"github.com/buildpacks/lifecycle/api"
	"github.com/docker/docker/client"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/heroku/color"
	"github.com/onsi/gomega/ghttp"
	"github.com/pkg/errors"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	"github.com/buildpacks/pack/config"
	"github.com/buildpacks/pack/internal/blob"
	"github.com/buildpacks/pack/internal/build"
	"github.com/buildpacks/pack/internal/builder"
	"github.com/buildpacks/pack/internal/buildpackage"
	cfg "github.com/buildpacks/pack/internal/config"
	"github.com/buildpacks/pack/internal/dist"
	ifakes "github.com/buildpacks/pack/internal/fakes"
	ilogging "github.com/buildpacks/pack/internal/logging"
	rg "github.com/buildpacks/pack/internal/registry"
	"github.com/buildpacks/pack/internal/style"
	"github.com/buildpacks/pack/project"
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
		subject                      *Client
		fakeImageFetcher             *ifakes.FakeImageFetcher
		fakeLifecycle                *ifakes.FakeLifecycle
		defaultBuilderStackID        = "some.stack.id"
		defaultWindowsBuilderStackID = "some.windows.stack.id"
		defaultBuilderImage          *fakes.Image
		defaultWindowsBuilderImage   *fakes.Image
		defaultBuilderName           = "example.com/default/builder:tag"
		defaultWindowsBuilderName    = "example.com/windows-default/builder:tag"
		defaultRunImageName          = "default/run"
		defaultWindowsRunImageName   = "default/win-run"
		fakeDefaultRunImage          *fakes.Image
		fakeDefaultWindowsRunImage   *fakes.Image
		fakeMirror1                  *fakes.Image
		fakeMirror2                  *fakes.Image
		tmpDir                       string
		outBuf                       bytes.Buffer
		logger                       *ilogging.LogWithWriters
		fakeLifecycleImage           *fakes.Image
	)
	it.Before(func() {
		var err error

		fakeImageFetcher = ifakes.NewFakeImageFetcher()
		fakeLifecycle = &ifakes.FakeLifecycle{}

		tmpDir, err = ioutil.TempDir("", "build-test")
		h.AssertNil(t, err)

		defaultBuilderImage = newFakeBuilderImage(t, tmpDir, defaultBuilderName, defaultBuilderStackID, defaultRunImageName, builder.DefaultLifecycleVersion, newLinuxImage)
		h.AssertNil(t, defaultBuilderImage.SetLabel("io.buildpacks.stack.mixins", `["mixinA", "build:mixinB", "mixinX", "build:mixinY"]`))
		fakeImageFetcher.LocalImages[defaultBuilderImage.Name()] = defaultBuilderImage

		defaultWindowsBuilderImage = newFakeBuilderImage(t, tmpDir, defaultWindowsBuilderName, defaultWindowsBuilderStackID, defaultWindowsRunImageName, builder.DefaultLifecycleVersion, newWindowsImage)
		h.AssertNil(t, defaultWindowsBuilderImage.SetLabel("io.buildpacks.stack.mixins", `["mixinA", "build:mixinB", "mixinX", "build:mixinY"]`))
		fakeImageFetcher.LocalImages[defaultWindowsBuilderImage.Name()] = defaultWindowsBuilderImage

		fakeDefaultWindowsRunImage = newWindowsImage("default/win-run", "", nil)
		h.AssertNil(t, fakeDefaultWindowsRunImage.SetLabel("io.buildpacks.stack.id", defaultWindowsBuilderStackID))
		h.AssertNil(t, fakeDefaultWindowsRunImage.SetLabel("io.buildpacks.stack.mixins", `["mixinA", "run:mixinC", "mixinX", "run:mixinZ"]`))
		fakeImageFetcher.LocalImages[fakeDefaultWindowsRunImage.Name()] = fakeDefaultWindowsRunImage

		fakeDefaultRunImage = newLinuxImage("default/run", "", nil)
		h.AssertNil(t, fakeDefaultRunImage.SetLabel("io.buildpacks.stack.id", defaultBuilderStackID))
		h.AssertNil(t, fakeDefaultRunImage.SetLabel("io.buildpacks.stack.mixins", `["mixinA", "run:mixinC", "mixinX", "run:mixinZ"]`))
		fakeImageFetcher.LocalImages[fakeDefaultRunImage.Name()] = fakeDefaultRunImage

		fakeMirror1 = newLinuxImage("registry1.example.com/run/mirror", "", nil)
		h.AssertNil(t, fakeMirror1.SetLabel("io.buildpacks.stack.id", defaultBuilderStackID))
		h.AssertNil(t, fakeMirror1.SetLabel("io.buildpacks.stack.mixins", `["mixinA", "mixinX", "run:mixinZ"]`))
		fakeImageFetcher.LocalImages[fakeMirror1.Name()] = fakeMirror1

		fakeMirror2 = newLinuxImage("registry2.example.com/run/mirror", "", nil)
		h.AssertNil(t, fakeMirror2.SetLabel("io.buildpacks.stack.id", defaultBuilderStackID))
		h.AssertNil(t, fakeMirror2.SetLabel("io.buildpacks.stack.mixins", `["mixinA", "mixinX", "run:mixinZ"]`))
		fakeImageFetcher.LocalImages[fakeMirror2.Name()] = fakeMirror2

		fakeLifecycleImage = newLinuxImage(fmt.Sprintf("%s:%s", cfg.DefaultLifecycleImageRepo, builder.DefaultLifecycleVersion), "", nil)
		fakeImageFetcher.LocalImages[fakeLifecycleImage.Name()] = fakeLifecycleImage

		docker, err := client.NewClientWithOpts(client.FromEnv, client.WithVersion("1.38"))
		h.AssertNil(t, err)

		logger = ilogging.NewLogWithWriters(&outBuf, &outBuf)

		dlCacheDir, err := ioutil.TempDir(tmpDir, "dl-cache")
		h.AssertNil(t, err)

		subject = &Client{
			logger:            logger,
			imageFetcher:      fakeImageFetcher,
			downloader:        blob.NewDownloader(logger, dlCacheDir),
			lifecycleExecutor: fakeLifecycle,
			docker:            docker,
		}
	})

	it.After(func() {
		h.AssertNilE(t, defaultBuilderImage.Cleanup())
		h.AssertNilE(t, fakeDefaultRunImage.Cleanup())
		h.AssertNilE(t, fakeMirror1.Cleanup())
		h.AssertNilE(t, fakeMirror2.Cleanup())
		os.RemoveAll(tmpDir)
		h.AssertNilE(t, fakeLifecycleImage.Cleanup())
	})

	when("#Build", func() {
		when("Workspace option", func() {
			it("uses the specified dir", func() {
				h.AssertNil(t, subject.Build(context.TODO(), BuildOptions{
					Workspace: "app",
					Builder:   defaultBuilderName,
					Image:     "example.com/some/repo:tag",
				}))
				h.AssertEq(t, fakeLifecycle.Opts.Workspace, "app")
			})
		})

		when("Image option", func() {
			it("is required", func() {
				h.AssertError(t, subject.Build(context.TODO(), BuildOptions{
					Image:   "",
					Builder: defaultBuilderName,
				}),
					"invalid image name ''",
				)
			})

			it("must be a valid image reference", func() {
				h.AssertError(t, subject.Build(context.TODO(), BuildOptions{
					Image:   "not@valid",
					Builder: defaultBuilderName,
				}),
					"invalid image name 'not@valid'",
				)
			})

			it("must be a valid tag reference", func() {
				h.AssertError(t, subject.Build(context.TODO(), BuildOptions{
					Image:   "registry.com/my/image@sha256:954e1f01e80ce09d0887ff6ea10b13a812cb01932a0781d6b0cc23f743a874fd",
					Builder: defaultBuilderName,
				}),
					"invalid image name 'registry.com/my/image@sha256:954e1f01e80ce09d0887ff6ea10b13a812cb01932a0781d6b0cc23f743a874fd'",
				)
			})

			it("lifecycle receives resolved reference", func() {
				h.AssertNil(t, subject.Build(context.TODO(), BuildOptions{
					Builder: defaultBuilderName,
					Image:   "example.com/some/repo:tag",
				}))
				h.AssertEq(t, fakeLifecycle.Opts.Image.Context().RegistryStr(), "example.com")
				h.AssertEq(t, fakeLifecycle.Opts.Image.Context().RepositoryStr(), "some/repo")
				h.AssertEq(t, fakeLifecycle.Opts.Image.Identifier(), "tag")
			})
		})

		when("Quiet mode", func() {
			var builtImage *fakes.Image

			it.After(func() {
				logger.WantQuiet(false)
			})

			when("publish", func() {
				var remoteRunImage, builderWithoutLifecycleImageOrCreator *fakes.Image

				it.Before(func() {
					remoteRunImage = fakes.NewImage("default/run", "", nil)
					h.AssertNil(t, remoteRunImage.SetLabel("io.buildpacks.stack.id", defaultBuilderStackID))
					h.AssertNil(t, remoteRunImage.SetLabel("io.buildpacks.stack.mixins", `["mixinA", "mixinX", "run:mixinZ"]`))
					fakeImageFetcher.RemoteImages[remoteRunImage.Name()] = remoteRunImage

					builderWithoutLifecycleImageOrCreator = newFakeBuilderImage(
						t,
						tmpDir,
						"example.com/supportscreator/builder:tag",
						"some.stack.id",
						defaultRunImageName,
						"0.3.0",
						newLinuxImage,
					)
					h.AssertNil(t, builderWithoutLifecycleImageOrCreator.SetLabel("io.buildpacks.stack.mixins", `["mixinA", "build:mixinB", "mixinX", "build:mixinY"]`))
					fakeImageFetcher.LocalImages[builderWithoutLifecycleImageOrCreator.Name()] = builderWithoutLifecycleImageOrCreator

					digest, err := name.NewDigest("example.io/some/app@sha256:363c754893f0efe22480b4359a5956cf3bd3ce22742fc576973c61348308c2e4", name.WeakValidation)
					h.AssertNil(t, err)
					builtImage = fakes.NewImage("example.io/some/app:latest", "", remote.DigestIdentifier{Digest: digest})
					fakeImageFetcher.RemoteImages[builtImage.Name()] = builtImage
				})

				it.After(func() {
					h.AssertNilE(t, remoteRunImage.Cleanup())
					h.AssertNilE(t, builderWithoutLifecycleImageOrCreator.Cleanup())
					h.AssertNilE(t, builtImage.Cleanup())
				})

				it("only prints app name and sha", func() {
					logger.WantQuiet(true)

					h.AssertNil(t, subject.Build(context.TODO(), BuildOptions{
						Image:   "example.io/some/app",
						Builder: defaultBuilderName,
						AppPath: filepath.Join("testdata", "some-app"),
						Publish: true,
					}))

					h.AssertEq(t, strings.TrimSpace(outBuf.String()), "example.io/some/app@sha256:363c754893f0efe22480b4359a5956cf3bd3ce22742fc576973c61348308c2e4")
				})
			})

			when("local", func() {
				it.Before(func() {
					builtImage = fakes.NewImage("index.docker.io/some/app:latest", "", local.IDIdentifier{
						ImageID: "363c754893f0efe22480b4359a5956cf3bd3ce22742fc576973c61348308c2e4",
					})
					fakeImageFetcher.LocalImages[builtImage.Name()] = builtImage
				})

				it.After(func() {
					h.AssertNilE(t, builtImage.Cleanup())
				})

				it("only prints app name and sha", func() {
					logger.WantQuiet(true)

					h.AssertNil(t, subject.Build(context.TODO(), BuildOptions{
						Image:   "some/app",
						Builder: defaultBuilderName,
						AppPath: filepath.Join("testdata", "some-app"),
					}))

					h.AssertEq(t, strings.TrimSpace(outBuf.String()), "some/app@sha256:363c754893f0efe22480b4359a5956cf3bd3ce22742fc576973c61348308c2e4")
				})
			})
		})

		when("AppDir option", func() {
			it("defaults to the current working directory", func() {
				h.AssertNil(t, subject.Build(context.TODO(), BuildOptions{
					Image:   "some/app",
					Builder: defaultBuilderName,
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
						Builder: defaultBuilderName,
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
						Builder: defaultBuilderName,
						AppPath: appPath,
					})

					h.AssertError(t, err, errMessage)
				})
			}

			it("resolves the absolute path", func() {
				h.AssertNil(t, subject.Build(context.TODO(), BuildOptions{
					Image:   "some/app",
					Builder: defaultBuilderName,
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
						Builder: defaultBuilderName,
						AppPath: relLink,
					}))

					h.AssertEq(t, fakeLifecycle.Opts.AppPath, absoluteAppDir)
				})

				it("resolves absolute symbolic links", func() {
					relLink := filepath.Join(tmpDir, "some-app.link")
					h.AssertNil(t, os.Symlink(absoluteAppDir, relLink))

					h.AssertNil(t, subject.Build(context.TODO(), BuildOptions{
						Image:   "some/app",
						Builder: defaultBuilderName,
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
						Builder: defaultBuilderName,
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
						defaultBuilderName,
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
										Version: *semver.MustParse(builder.DefaultLifecycleVersion),
									},
								},
								APIs: builder.LifecycleAPIs{
									Buildpack: builder.APIVersions{
										Supported: builder.APISet{api.MustParse("0.2"), api.MustParse("0.3"), api.MustParse("0.4")},
									},
									Platform: builder.APIVersions{
										Supported: builder.APISet{api.MustParse("0.3"), api.MustParse("0.4")},
									},
								},
							},
						},
						nil,
						nil,
						newLinuxImage,
					)

					fakeImageFetcher.LocalImages[customBuilderImage.Name()] = customBuilderImage

					fakeRunImage = fakes.NewImage("some/run", "", nil)
					h.AssertNil(t, fakeRunImage.SetLabel("io.buildpacks.stack.id", "some.stack.id"))
					fakeImageFetcher.LocalImages[fakeRunImage.Name()] = fakeRunImage
				})

				it.After(func() {
					h.AssertNilE(t, customBuilderImage.Cleanup())
					h.AssertNilE(t, fakeRunImage.Cleanup())
				})

				it("it uses the provided builder", func() {
					h.AssertNil(t, subject.Build(context.TODO(), BuildOptions{
						Image:   "some/app",
						Builder: defaultBuilderName,
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
				h.AssertNilE(t, fakeRunImage.Cleanup())
			})

			when("run image stack matches the builder stack", func() {
				it("uses the provided image", func() {
					h.AssertNil(t, subject.Build(context.TODO(), BuildOptions{
						Image:    "some/app",
						Builder:  defaultBuilderName,
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
						Builder:  defaultBuilderName,
						RunImage: "custom/run",
					}),
						"invalid run-image 'custom/run': run-image stack id 'other.stack' does not match builder stack 'some.stack.id'",
					)
				})
			})

			when("run image is not supplied", func() {
				when("there are no locally configured mirrors", func() {
					when("Publish is true", func() {
						it("chooses the run image mirror matching the local image", func() {
							fakeImageFetcher.RemoteImages[fakeDefaultRunImage.Name()] = fakeDefaultRunImage

							h.AssertNil(t, subject.Build(context.TODO(), BuildOptions{
								Image:   "some/app",
								Builder: defaultBuilderName,
								Publish: true,
							}))
							h.AssertEq(t, fakeLifecycle.Opts.RunImage, "default/run")
						})

						for _, registry := range []string{"registry1.example.com", "registry2.example.com"} {
							testRegistry := registry
							it("chooses the run image mirror matching the built image", func() {
								runImg := testRegistry + "/run/mirror"
								fakeImageFetcher.RemoteImages[runImg] = fakeDefaultRunImage
								h.AssertNil(t, subject.Build(context.TODO(), BuildOptions{
									Image:   testRegistry + "/some/app",
									Builder: defaultBuilderName,
									Publish: true,
								}))
								h.AssertEq(t, fakeLifecycle.Opts.RunImage, runImg)
							})
						}
					})

					when("Publish is false", func() {
						for _, img := range []string{"some/app",
							"registry1.example.com/some/app",
							"registry2.example.com/some/app"} {
							testImg := img
							it("chooses a mirror on the builder registry", func() {
								h.AssertNil(t, subject.Build(context.TODO(), BuildOptions{
									Image:   testImg,
									Builder: defaultBuilderName,
								}))
								h.AssertEq(t, fakeLifecycle.Opts.RunImage, "default/run")
							})
						}
					})
				})

				when("there are locally configured mirrors", func() {
					var (
						fakeLocalMirror  *fakes.Image
						fakeLocalMirror1 *fakes.Image
						mirrors          = map[string][]string{
							"default/run": {"local/mirror", "registry1.example.com/local/mirror"},
						}
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
						h.AssertNilE(t, fakeLocalMirror.Cleanup())
						h.AssertNilE(t, fakeLocalMirror1.Cleanup())
					})

					when("Publish is true", func() {
						for _, registry := range []string{"", "registry1.example.com"} {
							testRegistry := registry
							it("prefers user provided mirrors for registry "+testRegistry, func() {
								if testRegistry != "" {
									testRegistry += "/"
								}
								runImg := testRegistry + "local/mirror"
								fakeImageFetcher.RemoteImages[runImg] = fakeDefaultRunImage

								h.AssertNil(t, subject.Build(context.TODO(), BuildOptions{
									Image:             testRegistry + "some/app",
									Builder:           defaultBuilderName,
									AdditionalMirrors: mirrors,
									Publish:           true,
								}))
								h.AssertEq(t, fakeLifecycle.Opts.RunImage, runImg)
							})
						}
					})

					when("Publish is false", func() {
						for _, registry := range []string{"", "registry1.example.com", "registry2.example.com"} {
							testRegistry := registry
							it("prefers user provided mirrors", func() {
								h.AssertNil(t, subject.Build(context.TODO(), BuildOptions{
									Image:             testRegistry + "some/app",
									Builder:           defaultBuilderName,
									AdditionalMirrors: mirrors,
								}))
								h.AssertEq(t, fakeLifecycle.Opts.RunImage, "local/mirror")
							})
						}
					})
				})
			})
		})

		when("ClearCache option", func() {
			it("passes it through to lifecycle", func() {
				h.AssertNil(t, subject.Build(context.TODO(), BuildOptions{
					Image:      "some/app",
					Builder:    defaultBuilderName,
					ClearCache: true,
				}))
				h.AssertEq(t, fakeLifecycle.Opts.ClearCache, true)
			})

			it("defaults to false", func() {
				h.AssertNil(t, subject.Build(context.TODO(), BuildOptions{
					Image:   "some/app",
					Builder: defaultBuilderName,
				}))
				h.AssertEq(t, fakeLifecycle.Opts.ClearCache, false)
			})
		})

		when("ImageCache option", func() {
			it("passes it through to lifecycle", func() {
				h.AssertNil(t, subject.Build(context.TODO(), BuildOptions{
					Image:      "some/app",
					Builder:    defaultBuilderName,
					CacheImage: "some-cache-image",
				}))
				h.AssertEq(t, fakeLifecycle.Opts.CacheImage, "some-cache-image")
			})

			it("defaults to false", func() {
				h.AssertNil(t, subject.Build(context.TODO(), BuildOptions{
					Image:   "some/app",
					Builder: defaultBuilderName,
				}))
				h.AssertEq(t, fakeLifecycle.Opts.CacheImage, "")
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
				additionalBP := ifakes.CreateBuildpackTar(t, tmpDir, dist.BuildpackDescriptor{
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
					Builder:    defaultBuilderName,
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
						Builder:    defaultBuilderName,
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
						Builder:    defaultBuilderName,
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
					additionalBP1 := ifakes.CreateBuildpackTar(t, tmpDir, dist.BuildpackDescriptor{
						API: api.MustParse("0.3"),
						Info: dist.BuildpackInfo{
							ID:      "buildpack.add.1.id",
							Version: "buildpack.add.1.version",
						},
						Stacks: []dist.Stack{{ID: defaultBuilderStackID}},
						Order:  nil,
					})

					additionalBP2 := ifakes.CreateBuildpackTar(t, tmpDir, dist.BuildpackDescriptor{
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
						Builder:    defaultBuilderName,
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
					additionalBP1 := ifakes.CreateBuildpackTar(t, tmpDir, dist.BuildpackDescriptor{
						API: api.MustParse("0.3"),
						Info: dist.BuildpackInfo{
							ID:      "buildpack.add.1.id",
							Version: "buildpack.add.1.version",
						},
						Stacks: []dist.Stack{{ID: defaultBuilderStackID}},
						Order:  nil,
					})

					additionalBP2 := ifakes.CreateBuildpackTar(t, tmpDir, dist.BuildpackDescriptor{
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
						Builder:    defaultBuilderName,
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
					additionalBP1 := ifakes.CreateBuildpackTar(t, tmpDir, dist.BuildpackDescriptor{
						API: api.MustParse("0.3"),
						Info: dist.BuildpackInfo{
							ID:      "buildpack.add.1.id",
							Version: "buildpack.add.1.version",
						},
						Stacks: []dist.Stack{{ID: defaultBuilderStackID}},
						Order:  nil,
					})

					additionalBP2 := ifakes.CreateBuildpackTar(t, tmpDir, dist.BuildpackDescriptor{
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
						Builder:    defaultBuilderName,
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
					buildpackTar := ifakes.CreateBuildpackTar(t, tmpDir, dist.BuildpackDescriptor{
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
						Builder:    defaultBuilderName,
						ClearCache: true,
						Buildpacks: []string{buildpackTar},
					})

					h.AssertNil(t, err)
				})
			})

			when("buildpackage image is used", func() {
				var fakePackage *fakes.Image

				it.Before(func() {
					metaBuildpackTar := ifakes.CreateBuildpackTar(t, tmpDir, dist.BuildpackDescriptor{
						API: api.MustParse("0.3"),
						Info: dist.BuildpackInfo{
							ID:       "meta.buildpack.id",
							Version:  "meta.buildpack.version",
							Homepage: "http://meta.buildpack",
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

					childBuildpackTar := ifakes.CreateBuildpackTar(t, tmpDir, dist.BuildpackDescriptor{
						API: api.MustParse("0.3"),
						Info: dist.BuildpackInfo{
							ID:       "child.buildpack.id",
							Version:  "child.buildpack.version",
							Homepage: "http://child.buildpack",
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
						Builder:    defaultBuilderName,
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
						Builder:    defaultBuilderName,
						ClearCache: true,
						Buildpacks: []string{
							"example.com/some/package",
						},
					})

					h.AssertError(t, err, "extracting buildpacks from 'example.com/some/package': could not find label 'io.buildpacks.buildpackage.metadata'")
				})

				it("fails when no bp layers label is on package", func() {
					h.AssertNil(t, fakePackage.SetLabel("io.buildpacks.buildpack.layers", ""))

					err := subject.Build(context.TODO(), BuildOptions{
						Image:      "some/app",
						Builder:    defaultBuilderName,
						ClearCache: true,
						Buildpacks: []string{
							"example.com/some/package",
						},
					})

					h.AssertError(t, err, "extracting buildpacks from 'example.com/some/package': could not find label 'io.buildpacks.buildpack.layers'")
				})
			})

			it("ensures buildpacks exist on builder", func() {
				h.AssertError(t, subject.Build(context.TODO(), BuildOptions{
					Image:      "some/app",
					Builder:    defaultBuilderName,
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
					h.AssertNilE(t, os.Remove(buildpackTgz))
				})

				it("buildpacks are added to ephemeral builder", func() {
					err := subject.Build(context.TODO(), BuildOptions{
						Image:      "some/app",
						Builder:    defaultBuilderName,
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
					dirBuildpackInfo := dist.BuildpackInfo{ID: "bp.one", Version: "1.2.3", Homepage: "http://one.buildpack"}
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
							Builder:    defaultBuilderName,
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

					it("adds the buildpack form the project descriptor", func() {
						err := subject.Build(context.TODO(), BuildOptions{
							Image:      "some/app",
							Builder:    defaultBuilderName,
							ClearCache: true,
							ProjectDescriptor: project.Descriptor{
								Build: project.Build{
									Buildpacks: []project.Buildpack{{
										URI: server.URL(),
									}},
								},
							},
						})

						h.AssertNil(t, err)
						h.AssertEq(t, fakeLifecycle.Opts.Builder.Name(), defaultBuilderImage.Name())
						bldr, err := builder.FromImage(defaultBuilderImage)
						h.AssertNil(t, err)
						h.AssertEq(t, bldr.Order(), dist.Order{
							{Group: []dist.BuildpackRef{
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
							Builder: defaultBuilderName,
							Buildpacks: []string{
								buildpackTgz, // requires mixinA, build:mixinB, run:mixinC
							},
						})

						h.AssertError(t, err, "validating stack mixins: buildpack 'some-other-buildpack-id@some-other-buildpack-version' requires missing mixin(s): build:mixinB, mixinA, run:mixinC")
					})
				})

				when("buildpack is inline", func() {
					var (
						tmpDir string
					)

					it.Before(func() {
						var err error
						tmpDir, err = ioutil.TempDir("", "project-desc")
						h.AssertNil(t, err)
					})

					it.After(func() {
						err := os.RemoveAll(tmpDir)
						h.AssertNil(t, err)
					})

					it("all buildpacks are added to ephemeral builder", func() {
						err := subject.Build(context.TODO(), BuildOptions{
							Image:      "some/app",
							Builder:    defaultBuilderName,
							ClearCache: true,
							ProjectDescriptor: project.Descriptor{
								Build: project.Build{
									Buildpacks: []project.Buildpack{{
										ID: "my/inline",
										Script: project.Script{
											API:    "0.4",
											Inline: "touch foo.txt",
										},
									}},
								},
							},
							ProjectDescriptorBaseDir: tmpDir,
						})

						h.AssertNil(t, err)
						h.AssertEq(t, fakeLifecycle.Opts.Builder.Name(), defaultBuilderImage.Name())
						bldr, err := builder.FromImage(defaultBuilderImage)
						h.AssertNil(t, err)
						h.AssertEq(t, bldr.Order(), dist.Order{
							{Group: []dist.BuildpackRef{
								{BuildpackInfo: dist.BuildpackInfo{ID: "my/inline", Version: "0.0.0"}},
							}},
						})
						h.AssertEq(t, bldr.Buildpacks(), []dist.BuildpackInfo{
							{ID: "buildpack.1.id", Version: "buildpack.1.version"},
							{ID: "buildpack.2.id", Version: "buildpack.2.version"},
							{ID: "my/inline", Version: "0.0.0"},
						})
					})

					it("fails if there is no API", func() {
						err := subject.Build(context.TODO(), BuildOptions{
							Image:      "some/app",
							Builder:    defaultBuilderName,
							ClearCache: true,
							ProjectDescriptor: project.Descriptor{
								Build: project.Build{
									Buildpacks: []project.Buildpack{{
										ID: "my/inline",
										Script: project.Script{
											Inline: "touch foo.txt",
										},
									}},
								},
							},
							ProjectDescriptorBaseDir: tmpDir,
						})

						h.AssertEq(t, "Missing API version for inline buildpack", err.Error())
					})

					it("fails if there is no ID", func() {
						err := subject.Build(context.TODO(), BuildOptions{
							Image:      "some/app",
							Builder:    defaultBuilderName,
							ClearCache: true,
							ProjectDescriptor: project.Descriptor{
								Build: project.Build{
									Buildpacks: []project.Buildpack{{
										Script: project.Script{
											API:    "0.4",
											Inline: "touch foo.txt",
										},
									}},
								},
							},
							ProjectDescriptorBaseDir: tmpDir,
						})

						h.AssertEq(t, "Invalid buildpack defined in project descriptor", err.Error())
					})

					it("ignores script if there is an id and version", func() {
						err := subject.Build(context.TODO(), BuildOptions{
							Image:      "some/app",
							Builder:    defaultBuilderName,
							ClearCache: true,
							ProjectDescriptor: project.Descriptor{
								Build: project.Build{
									Buildpacks: []project.Buildpack{{
										ID:      "buildpack.1.id",
										Version: "buildpack.1.version",
										Script: project.Script{
											Inline: "touch foo.txt",
										},
									}},
								},
							},
							ProjectDescriptorBaseDir: tmpDir,
						})

						h.AssertNil(t, err)
						h.AssertEq(t, fakeLifecycle.Opts.Builder.Name(), defaultBuilderImage.Name())
						bldr, err := builder.FromImage(defaultBuilderImage)
						h.AssertNil(t, err)
						h.AssertEq(t, bldr.Order(), dist.Order{
							{Group: []dist.BuildpackRef{
								{BuildpackInfo: dist.BuildpackInfo{ID: "buildpack.1.id", Version: "buildpack.1.version"}},
							}},
						})
						h.AssertEq(t, bldr.Buildpacks(), []dist.BuildpackInfo{
							{ID: "buildpack.1.id", Version: "buildpack.1.version"},
							{ID: "buildpack.2.id", Version: "buildpack.2.version"},
						})
					})
				})

				when("buildpack is from a registry", func() {
					var (
						fakePackage     *fakes.Image
						tmpDir          string
						registryFixture string
						packHome        string

						configPath string
					)

					it.Before(func() {
						var err error
						tmpDir, err = ioutil.TempDir("", "registry")
						h.AssertNil(t, err)

						packHome = filepath.Join(tmpDir, ".pack")
						err = os.MkdirAll(packHome, 0755)
						h.AssertNil(t, err)
						os.Setenv("PACK_HOME", packHome)

						registryFixture = h.CreateRegistryFixture(t, tmpDir, filepath.Join("testdata", "registry"))

						configPath = filepath.Join(packHome, "config.toml")
						h.AssertNil(t, cfg.Write(cfg.Config{
							Registries: []cfg.Registry{
								{
									Name: "some-registry",
									Type: "github",
									URL:  registryFixture,
								},
							},
						}, configPath))

						_, err = rg.NewRegistryCache(logger, tmpDir, registryFixture)
						h.AssertNil(t, err)

						childBuildpackTar := ifakes.CreateBuildpackTar(t, tmpDir, dist.BuildpackDescriptor{
							API: api.MustParse("0.3"),
							Info: dist.BuildpackInfo{
								ID:      "example/foo",
								Version: "1.0.0",
							},
							Stacks: []dist.Stack{
								{ID: defaultBuilderStackID},
							},
						})

						bpLayers := dist.BuildpackLayers{
							"example/foo": {
								"1.0.0": {
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
								ID:      "example/foo",
								Version: "1.0.0",
							},
							Stacks: []dist.Stack{
								{ID: defaultBuilderStackID},
							},
						}

						fakePackage = fakes.NewImage("example.com/some/package@sha256:8c27fe111c11b722081701dfed3bd55e039b9ce92865473cf4cdfa918071c566", "", nil)
						h.AssertNil(t, dist.SetLabel(fakePackage, "io.buildpacks.buildpack.layers", bpLayers))
						h.AssertNil(t, dist.SetLabel(fakePackage, "io.buildpacks.buildpackage.metadata", md))

						h.AssertNil(t, fakePackage.AddLayer(childBuildpackTar))

						fakeImageFetcher.LocalImages[fakePackage.Name()] = fakePackage
					})

					it.After(func() {
						os.Unsetenv("PACK_HOME")
						h.AssertNil(t, os.RemoveAll(tmpDir))
					})

					it("all buildpacks are added to ephemeral builder", func() {
						err := subject.Build(context.TODO(), BuildOptions{
							Image:      "some/app",
							Builder:    defaultBuilderName,
							ClearCache: true,
							Buildpacks: []string{
								"urn:cnb:registry:example/foo@1.0.0",
							},
							Registry: "some-registry",
						})

						h.AssertNil(t, err)
						h.AssertEq(t, fakeLifecycle.Opts.Builder.Name(), defaultBuilderImage.Name())
						bldr, err := builder.FromImage(defaultBuilderImage)
						h.AssertNil(t, err)
						h.AssertEq(t, bldr.Order(), dist.Order{
							{Group: []dist.BuildpackRef{
								{BuildpackInfo: dist.BuildpackInfo{ID: "example/foo", Version: "1.0.0"}},
							}},
						})
						h.AssertEq(t, bldr.Buildpacks(), []dist.BuildpackInfo{
							{ID: "buildpack.1.id", Version: "buildpack.1.version"},
							{ID: "buildpack.2.id", Version: "buildpack.2.version"},
							{ID: "example/foo", Version: "1.0.0"},
						})
					})
				})
			})
		})

		when("Env option", func() {
			it("should set the env on the ephemeral builder", func() {
				h.AssertNil(t, subject.Build(context.TODO(), BuildOptions{
					Image:   "some/app",
					Builder: defaultBuilderName,
					Env: map[string]string{
						"key1": "value1",
						"key2": "value2",
					},
				}))
				layerTar, err := defaultBuilderImage.FindLayerWithPath("/platform/env/key1")
				h.AssertNil(t, err)
				h.AssertTarFileContents(t, layerTar, "/platform/env/key1", `value1`)
				h.AssertTarFileContents(t, layerTar, "/platform/env/key2", `value2`)
			})
		})

		when("Publish option", func() {
			var remoteRunImage, builderWithoutLifecycleImageOrCreator *fakes.Image

			it.Before(func() {
				remoteRunImage = fakes.NewImage("default/run", "", nil)
				h.AssertNil(t, remoteRunImage.SetLabel("io.buildpacks.stack.id", defaultBuilderStackID))
				h.AssertNil(t, remoteRunImage.SetLabel("io.buildpacks.stack.mixins", `["mixinA", "mixinX", "run:mixinZ"]`))
				fakeImageFetcher.RemoteImages[remoteRunImage.Name()] = remoteRunImage

				builderWithoutLifecycleImageOrCreator = newFakeBuilderImage(
					t,
					tmpDir,
					"example.com/supportscreator/builder:tag",
					"some.stack.id",
					defaultRunImageName,
					"0.3.0",
					newLinuxImage,
				)
				h.AssertNil(t, builderWithoutLifecycleImageOrCreator.SetLabel("io.buildpacks.stack.mixins", `["mixinA", "build:mixinB", "mixinX", "build:mixinY"]`))
				fakeImageFetcher.LocalImages[builderWithoutLifecycleImageOrCreator.Name()] = builderWithoutLifecycleImageOrCreator
			})

			it.After(func() {
				remoteRunImage.Cleanup()
				builderWithoutLifecycleImageOrCreator.Cleanup()
			})

			when("true", func() {
				it("uses a remote run image", func() {
					h.AssertNil(t, subject.Build(context.TODO(), BuildOptions{
						Image:   "some/app",
						Builder: defaultBuilderName,
						Publish: true,
					}))
					h.AssertEq(t, fakeLifecycle.Opts.Publish, true)

					args := fakeImageFetcher.FetchCalls["default/run"]
					h.AssertEq(t, args.Daemon, false)

					args = fakeImageFetcher.FetchCalls[defaultBuilderName]
					h.AssertEq(t, args.Daemon, true)
				})

				when("builder is untrusted", func() {
					when("lifecycle image is available", func() {
						it("uses the 5 phases with the lifecycle image", func() {
							h.AssertNil(t, subject.Build(context.TODO(), BuildOptions{
								Image:        "some/app",
								Builder:      defaultBuilderName,
								Publish:      true,
								TrustBuilder: false,
							}))
							h.AssertEq(t, fakeLifecycle.Opts.UseCreator, false)
							h.AssertEq(t, fakeLifecycle.Opts.LifecycleImage, fakeLifecycleImage.Name())

							args := fakeImageFetcher.FetchCalls[fakeLifecycleImage.Name()]
							h.AssertEq(t, args.Daemon, true)
							h.AssertEq(t, args.PullPolicy, config.PullAlways)
							h.AssertEq(t, args.Platform, "linux/amd64")
						})
					})

					when("lifecycle image is not available", func() {
						it("errors", func() {
							h.AssertNotNil(t, subject.Build(context.TODO(), BuildOptions{
								Image:        "some/app",
								Builder:      builderWithoutLifecycleImageOrCreator.Name(),
								Publish:      true,
								TrustBuilder: false,
							}))
						})
					})
				})

				when("builder is trusted", func() {
					when("lifecycle supports creator", func() {
						it("uses the creator with the provided builder", func() {
							h.AssertNil(t, subject.Build(context.TODO(), BuildOptions{
								Image:        "some/app",
								Builder:      defaultBuilderName,
								Publish:      true,
								TrustBuilder: true,
							}))
							h.AssertEq(t, fakeLifecycle.Opts.UseCreator, true)

							args := fakeImageFetcher.FetchCalls[fakeLifecycleImage.Name()]
							h.AssertNil(t, args)
						})
					})

					when("lifecycle doesn't support creator", func() {
						// the default test builder (example.com/default/builder:tag) has lifecycle version 0.3.0, so creator is not supported
						it("uses the 5 phases with the provided builder", func() {
							h.AssertNil(t, subject.Build(context.TODO(), BuildOptions{
								Image:        "some/app",
								Builder:      builderWithoutLifecycleImageOrCreator.Name(),
								Publish:      true,
								TrustBuilder: true,
							}))
							h.AssertEq(t, fakeLifecycle.Opts.UseCreator, false)
							h.AssertEq(t, fakeLifecycle.Opts.LifecycleImage, builderWithoutLifecycleImageOrCreator.Name())

							args := fakeImageFetcher.FetchCalls[fakeLifecycleImage.Name()]
							h.AssertNil(t, args)
						})
					})
				})
			})

			when("false", func() {
				it("uses a local run image", func() {
					h.AssertNil(t, subject.Build(context.TODO(), BuildOptions{
						Image:   "some/app",
						Builder: defaultBuilderName,
						Publish: false,
					}))
					h.AssertEq(t, fakeLifecycle.Opts.Publish, false)

					args := fakeImageFetcher.FetchCalls["default/run"]
					h.AssertEq(t, args.Daemon, true)
					h.AssertEq(t, args.PullPolicy, config.PullAlways)

					args = fakeImageFetcher.FetchCalls[defaultBuilderName]
					h.AssertEq(t, args.Daemon, true)
					h.AssertEq(t, args.PullPolicy, config.PullAlways)
				})

				when("builder is untrusted", func() {
					when("lifecycle image is available", func() {
						it("uses the 5 phases with the lifecycle image", func() {
							h.AssertNil(t, subject.Build(context.TODO(), BuildOptions{
								Image:        "some/app",
								Builder:      defaultBuilderName,
								Publish:      false,
								TrustBuilder: false,
							}))
							h.AssertEq(t, fakeLifecycle.Opts.UseCreator, false)
							h.AssertEq(t, fakeLifecycle.Opts.LifecycleImage, fakeLifecycleImage.Name())

							args := fakeImageFetcher.FetchCalls[fakeLifecycleImage.Name()]
							h.AssertEq(t, args.Daemon, true)
							h.AssertEq(t, args.PullPolicy, config.PullAlways)
							h.AssertEq(t, args.Platform, "linux/amd64")
						})

						it("suggests that being untrusted may be the root of a failure", func() {
							subject.lifecycleExecutor = &executeFailsLifecycle{}
							err := subject.Build(context.TODO(), BuildOptions{
								Image:        "some/app",
								Builder:      defaultBuilderName,
								Publish:      false,
								TrustBuilder: false,
							})

							h.AssertError(t, err, "may be the result of using an untrusted builder")
						})
					})

					when("lifecycle image is not available", func() {
						it("errors", func() {
							h.AssertNotNil(t, subject.Build(context.TODO(), BuildOptions{
								Image:        "some/app",
								Builder:      builderWithoutLifecycleImageOrCreator.Name(),
								Publish:      false,
								TrustBuilder: false,
							}))
						})
					})
				})

				when("builder is trusted", func() {
					when("lifecycle supports creator", func() {
						it("uses the creator with the provided builder", func() {
							h.AssertNil(t, subject.Build(context.TODO(), BuildOptions{
								Image:        "some/app",
								Builder:      defaultBuilderName,
								Publish:      false,
								TrustBuilder: true,
							}))
							h.AssertEq(t, fakeLifecycle.Opts.UseCreator, true)

							args := fakeImageFetcher.FetchCalls[fakeLifecycleImage.Name()]
							h.AssertNil(t, args)
						})
					})

					when("lifecycle doesn't support creator", func() {
						// the default test builder (example.com/default/builder:tag) has lifecycle version 0.3.0, so creator is not supported
						it("uses the 5 phases with the provided builder", func() {
							h.AssertNil(t, subject.Build(context.TODO(), BuildOptions{
								Image:        "some/app",
								Builder:      builderWithoutLifecycleImageOrCreator.Name(),
								Publish:      false,
								TrustBuilder: true,
							}))
							h.AssertEq(t, fakeLifecycle.Opts.UseCreator, false)
							h.AssertEq(t, fakeLifecycle.Opts.LifecycleImage, builderWithoutLifecycleImageOrCreator.Name())

							args := fakeImageFetcher.FetchCalls[fakeLifecycleImage.Name()]
							h.AssertNil(t, args)
						})
					})
				})
			})
		})

		when("PullPolicy", func() {
			when("never", func() {
				it("uses the local builder and run images without updating", func() {
					h.AssertNil(t, subject.Build(context.TODO(), BuildOptions{
						Image:      "some/app",
						Builder:    defaultBuilderName,
						PullPolicy: config.PullNever,
					}))

					args := fakeImageFetcher.FetchCalls["default/run"]
					h.AssertEq(t, args.Daemon, true)
					h.AssertEq(t, args.PullPolicy, config.PullNever)

					args = fakeImageFetcher.FetchCalls[defaultBuilderName]
					h.AssertEq(t, args.Daemon, true)
					h.AssertEq(t, args.PullPolicy, config.PullNever)

					args = fakeImageFetcher.FetchCalls["buildpacksio/lifecycle:0.11.3"]
					h.AssertEq(t, args.Daemon, true)
					h.AssertEq(t, args.PullPolicy, config.PullNever)
					h.AssertEq(t, args.Platform, "linux/amd64")
				})
			})

			when("always", func() {
				it("uses pulls the builder and run image before using them", func() {
					h.AssertNil(t, subject.Build(context.TODO(), BuildOptions{
						Image:      "some/app",
						Builder:    defaultBuilderName,
						PullPolicy: config.PullAlways,
					}))

					args := fakeImageFetcher.FetchCalls["default/run"]
					h.AssertEq(t, args.Daemon, true)
					h.AssertEq(t, args.PullPolicy, config.PullAlways)

					args = fakeImageFetcher.FetchCalls[defaultBuilderName]
					h.AssertEq(t, args.Daemon, true)
					h.AssertEq(t, args.PullPolicy, config.PullAlways)
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
						h.AssertNilE(t, os.Unsetenv("HTTP_PROXY"))
						h.AssertNilE(t, os.Unsetenv("HTTPS_PROXY"))
						h.AssertNilE(t, os.Unsetenv("NO_PROXY"))
					})

					it("defaults to the *_PROXY environment variables", func() {
						h.AssertNil(t, subject.Build(context.TODO(), BuildOptions{
							Image:   "some/app",
							Builder: defaultBuilderName,
						}))
						h.AssertEq(t, fakeLifecycle.Opts.HTTPProxy, "some-http-proxy")
						h.AssertEq(t, fakeLifecycle.Opts.HTTPSProxy, "some-https-proxy")
						h.AssertEq(t, fakeLifecycle.Opts.NoProxy, "some-no-proxy")
					})
				})

				it("falls back to the *_proxy environment variables", func() {
					h.AssertNil(t, subject.Build(context.TODO(), BuildOptions{
						Image:   "some/app",
						Builder: defaultBuilderName,
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
						Builder: defaultBuilderName,
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
					Builder: defaultBuilderName,
					ContainerConfig: ContainerConfig{
						Network: "some-network",
					},
				}))
				h.AssertEq(t, fakeLifecycle.Opts.Network, "some-network")
			})
		})

		when("Lifecycle option", func() {
			when("Platform API", func() {
				for _, supportedPlatformAPI := range []string{"0.3", "0.4"} {
					var (
						supportedPlatformAPI = supportedPlatformAPI
						compatibleBuilder    *fakes.Image
					)

					when(fmt.Sprintf("lifecycle platform API is compatible (%s)", supportedPlatformAPI), func() {
						it.Before(func() {
							compatibleBuilder = ifakes.NewFakeBuilderImage(t,
								tmpDir,
								"compatible-"+defaultBuilderName,
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
												Version: *semver.MustParse(builder.DefaultLifecycleVersion),
											},
										},
										APIs: builder.LifecycleAPIs{
											Buildpack: builder.APIVersions{
												Supported: builder.APISet{api.MustParse("0.2"), api.MustParse("0.3"), api.MustParse("0.4")},
											},
											Platform: builder.APIVersions{
												Supported: builder.APISet{api.MustParse(supportedPlatformAPI)},
											},
										},
									},
								},
								nil,
								nil,
								newLinuxImage,
							)

							fakeImageFetcher.LocalImages[compatibleBuilder.Name()] = compatibleBuilder
						})

						it("should succeed", func() {
							err := subject.Build(context.TODO(), BuildOptions{
								Image:   "some/app",
								Builder: compatibleBuilder.Name(),
							})

							h.AssertNil(t, err)
						})
					})
				}

				when("lifecycle Platform API is not compatible", func() {
					var incompatibleBuilderImage *fakes.Image
					it.Before(func() {
						incompatibleBuilderImage = ifakes.NewFakeBuilderImage(t,
							tmpDir,
							"incompatible-"+defaultBuilderName,
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
											Version: *semver.MustParse(builder.DefaultLifecycleVersion),
										},
									},
									API: builder.LifecycleAPI{
										BuildpackVersion: api.MustParse("0.3"),
										PlatformVersion:  api.MustParse("0.1"),
									},
								},
							},
							nil,
							nil,
							newLinuxImage,
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

				when("supported Platform APIs not specified", func() {
					var badBuilderImage *fakes.Image
					it.Before(func() {
						badBuilderImage = ifakes.NewFakeBuilderImage(t,
							tmpDir,
							"incompatible-"+defaultBuilderName,
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
											Version: *semver.MustParse(builder.DefaultLifecycleVersion),
										},
									},
									APIs: builder.LifecycleAPIs{
										Buildpack: builder.APIVersions{Supported: builder.APISet{api.MustParse("0.2")}},
									},
								},
							},
							nil,
							nil,
							newLinuxImage,
						)

						fakeImageFetcher.LocalImages[badBuilderImage.Name()] = badBuilderImage
					})

					it.After(func() {
						badBuilderImage.Cleanup()
					})

					it("should error", func() {
						builderName := badBuilderImage.Name()

						err := subject.Build(context.TODO(), BuildOptions{
							Image:   "some/app",
							Builder: builderName,
						})

						h.AssertError(t, err, "supported Lifecycle Platform APIs not specified")
					})
				})
			})

			when("Buildpack API", func() {
				when("supported Buildpack APIs not specified", func() {
					var badBuilderImage *fakes.Image
					it.Before(func() {
						badBuilderImage = ifakes.NewFakeBuilderImage(t,
							tmpDir,
							"incompatible-"+defaultBuilderName,
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
											Version: *semver.MustParse(builder.DefaultLifecycleVersion),
										},
									},
									APIs: builder.LifecycleAPIs{
										Platform: builder.APIVersions{Supported: builder.APISet{api.MustParse("0.4")}},
									},
								},
							},
							nil,
							nil,
							newLinuxImage,
						)

						fakeImageFetcher.LocalImages[badBuilderImage.Name()] = badBuilderImage
					})

					it.After(func() {
						badBuilderImage.Cleanup()
					})

					it("should error", func() {
						builderName := badBuilderImage.Name()

						err := subject.Build(context.TODO(), BuildOptions{
							Image:   "some/app",
							Builder: builderName,
						})

						h.AssertError(t, err, "supported Lifecycle Buildpack APIs not specified")
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
						Builder: defaultBuilderName,
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
						Builder: defaultBuilderName,
					})

					h.AssertError(t, err, "validating stack mixins: buildpack 'buildpack.1.id@buildpack.1.version' requires missing mixin(s): build:mixinY, mixinX, run:mixinZ")
				})
			})
		})

		when("Volumes option", func() {
			when("on posix", func() {
				it.Before(func() {
					h.SkipIf(t, runtime.GOOS == "windows", "Skipped on windows")
				})

				for _, test := range []struct {
					name        string
					volume      string
					expectation string
				}{
					{"defaults to read-only", "/a:/x", "/a:/x:ro"},
					{"defaults to read-only (nested)", "/a:/some/path/y", "/a:/some/path/y:ro"},
					{"supports rw mode", "/a:/x:rw", "/a:/x:rw"},
				} {
					volume := test.volume
					expectation := test.expectation

					it(test.name, func() {
						err := subject.Build(context.TODO(), BuildOptions{
							Image:   "some/app",
							Builder: defaultBuilderName,
							ContainerConfig: ContainerConfig{
								Volumes: []string{volume},
							},
						})
						h.AssertNil(t, err)
						h.AssertEq(t, fakeLifecycle.Opts.Volumes, []string{expectation})
					})
				}

				when("volume mode is invalid", func() {
					it("returns an error", func() {
						err := subject.Build(context.TODO(), BuildOptions{
							Image:   "some/app",
							Builder: defaultBuilderName,
							ContainerConfig: ContainerConfig{
								Volumes: []string{"/a:/x:invalid"},
							},
						})
						h.AssertError(t, err, `platform volume "/a:/x:invalid" has invalid format: invalid mode: invalid`)
					})
				})

				when("volume specification is invalid", func() {
					it("returns an error", func() {
						err := subject.Build(context.TODO(), BuildOptions{
							Image:   "some/app",
							Builder: defaultBuilderName,
							ContainerConfig: ContainerConfig{
								Volumes: []string{":::"},
							},
						})
						h.AssertError(t, err, `platform volume ":::" has invalid format: invalid volume specification: ':::'`)
					})
				})

				when("mounting onto cnb spec'd dir", func() {
					for _, p := range []string{
						"/cnb/buildpacks",
						"/cnb/buildpacks/nested",
						"/cnb",
						"/cnb/nested",
						"/layers",
						"/layers/nested",
					} {
						p := p
						it(fmt.Sprintf("warns when mounting to '%s'", p), func() {
							err := subject.Build(context.TODO(), BuildOptions{
								Image:   "some/app",
								Builder: defaultBuilderName,
								ContainerConfig: ContainerConfig{
									Volumes: []string{fmt.Sprintf("/tmp/path:%s", p)},
								},
							})

							h.AssertNil(t, err)
							h.AssertContains(t, outBuf.String(), fmt.Sprintf("Warning: Mounting to a sensitive directory '%s'", p))
						})
					}
				})
			})

			when("on windows", func() {
				it.Before(func() {
					h.SkipIf(t, runtime.GOOS != "windows", "Skipped on non-windows")
				})
				when("linux container", func() {
					it("drive is transformed", func() {
						dir, _ := ioutil.TempDir("", "pack-test-mount")
						volume := fmt.Sprintf("%v:/x", dir)
						err := subject.Build(context.TODO(), BuildOptions{
							Image:   "some/app",
							Builder: defaultBuilderName,
							ContainerConfig: ContainerConfig{
								Volumes: []string{volume},
							},
							TrustBuilder: true,
						})
						expected := []string{
							fmt.Sprintf("%s:/x:ro", strings.ToLower(dir)),
						}
						h.AssertNil(t, err)
						h.AssertEq(t, fakeLifecycle.Opts.Volumes, expected)
					})

					// May not fail as mode is not used on Windows
					when("volume mode is invalid", func() {
						it("returns an error", func() {
							err := subject.Build(context.TODO(), BuildOptions{
								Image:   "some/app",
								Builder: defaultBuilderName,
								ContainerConfig: ContainerConfig{
									Volumes: []string{"/a:/x:invalid"},
								},
								TrustBuilder: true,
							})
							h.AssertError(t, err, `platform volume "/a:/x:invalid" has invalid format: invalid volume specification: '/a:/x:invalid'`)
						})
					})

					when("volume specification is invalid", func() {
						it("returns an error", func() {
							err := subject.Build(context.TODO(), BuildOptions{
								Image:   "some/app",
								Builder: defaultBuilderName,
								ContainerConfig: ContainerConfig{
									Volumes: []string{":::"},
								},
								TrustBuilder: true,
							})
							h.AssertError(t, err, `platform volume ":::" has invalid format: invalid volume specification: ':::'`)
						})
					})

					when("mounting onto cnb spec'd dir", func() {
						for _, p := range []string{
							`/cnb`, `/cnb/buildpacks`, `/layers`,
						} {
							p := p
							it(fmt.Sprintf("warns when mounting to '%s'", p), func() {
								err := subject.Build(context.TODO(), BuildOptions{
									Image:   "some/app",
									Builder: defaultBuilderName,
									ContainerConfig: ContainerConfig{
										Volumes: []string{fmt.Sprintf("c:/Users:%s", p)},
									},
									TrustBuilder: true,
								})

								h.AssertNil(t, err)
								h.AssertContains(t, outBuf.String(), fmt.Sprintf("Warning: Mounting to a sensitive directory '%s'", p))
							})
						}
					})
				})
				when("windows container", func() {
					it("drive is mounted", func() {
						dir, _ := ioutil.TempDir("", "pack-test-mount")
						volume := fmt.Sprintf("%v:c:\\x", dir)
						err := subject.Build(context.TODO(), BuildOptions{
							Image:   "some/app",
							Builder: defaultWindowsBuilderName,
							ContainerConfig: ContainerConfig{
								Volumes: []string{volume},
							},
							TrustBuilder: true,
						})
						expected := []string{
							fmt.Sprintf("%s:c:\\x:ro", strings.ToLower(dir)),
						}
						h.AssertNil(t, err)
						h.AssertEq(t, fakeLifecycle.Opts.Volumes, expected)
					})

					// May not fail as mode is not used on Windows
					when("volume mode is invalid", func() {
						it("returns an error", func() {
							err := subject.Build(context.TODO(), BuildOptions{
								Image:   "some/app",
								Builder: defaultWindowsBuilderName,
								ContainerConfig: ContainerConfig{
									Volumes: []string{"/a:/x:invalid"},
								},
								TrustBuilder: true,
							})
							h.AssertError(t, err, `platform volume "/a:/x:invalid" has invalid format: invalid volume specification: '/a:/x:invalid'`)
						})
					})

					// Should fail even on windows
					when("volume specification is invalid", func() {
						it("returns an error", func() {
							err := subject.Build(context.TODO(), BuildOptions{
								Image:   "some/app",
								Builder: defaultWindowsBuilderName,
								ContainerConfig: ContainerConfig{
									Volumes: []string{":::"},
								},
								TrustBuilder: true,
							})
							h.AssertError(t, err, `platform volume ":::" has invalid format: invalid volume specification: ':::'`)
						})
					})

					when("mounting onto cnb spec'd dir", func() {
						for _, p := range []string{
							`c:\cnb`, `c:\cnb\buildpacks`, `c:\layers`,
						} {
							p := p
							it(fmt.Sprintf("warns when mounting to '%s'", p), func() {
								err := subject.Build(context.TODO(), BuildOptions{
									Image:   "some/app",
									Builder: defaultWindowsBuilderName,
									ContainerConfig: ContainerConfig{
										Volumes: []string{fmt.Sprintf("c:/Users:%s", p)},
									},
									TrustBuilder: true,
								})

								h.AssertNil(t, err)
								h.AssertContains(t, outBuf.String(), fmt.Sprintf("Warning: Mounting to a sensitive directory '%s'", p))
							})
						}
					})
				})
			})
		})

		when("gid option", func() {
			it("gid is passthroughs to lifecycle", func() {
				h.AssertNil(t, subject.Build(context.TODO(), BuildOptions{
					Workspace: "app",
					Builder:   defaultBuilderName,
					Image:     "example.com/some/repo:tag",
					GroupID:   2,
				}))
				h.AssertEq(t, fakeLifecycle.Opts.GID, 2)
			})
		})

		when("RegistryMirrors option", func() {
			it("translates run image before passing to lifecycle", func() {
				subject.registryMirrors = map[string]string{
					"index.docker.io": "10.0.0.1",
				}

				h.AssertNil(t, subject.Build(context.TODO(), BuildOptions{
					Builder: defaultBuilderName,
					Image:   "example.com/some/repo:tag",
				}))
				h.AssertEq(t, fakeLifecycle.Opts.RunImage, "10.0.0.1/default/run:latest")
			})
		})
	})
}

func diffIDForFile(t *testing.T, path string) string {
	file, err := os.Open(path)
	h.AssertNil(t, err)

	hasher := sha256.New()
	_, err = io.Copy(hasher, file)
	h.AssertNil(t, err)

	return "sha256:" + hex.EncodeToString(hasher.Sum(make([]byte, 0, hasher.Size())))
}

func newLinuxImage(name, topLayerSha string, identifier imgutil.Identifier) *fakes.Image {
	return fakes.NewImage(name, topLayerSha, identifier)
}

func newWindowsImage(name, topLayerSha string, identifier imgutil.Identifier) *fakes.Image {
	result := fakes.NewImage(name, topLayerSha, identifier)
	arch, _ := result.Architecture()
	osVersion, _ := result.OSVersion()
	result.SetOS("windows")
	result.SetOSVersion(osVersion)
	result.SetArchitecture(arch)
	return result
}

func newFakeBuilderImage(t *testing.T, tmpDir, builderName, defaultBuilderStackID, runImageName, lifecycleVersion string, osImageCreator ifakes.FakeImageCreator) *fakes.Image {
	return ifakes.NewFakeBuilderImage(t,
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
					Image: runImageName,
					Mirrors: []string{
						"registry1.example.com/run/mirror",
						"registry2.example.com/run/mirror",
					},
				},
			},
			Lifecycle: builder.LifecycleMetadata{
				LifecycleInfo: builder.LifecycleInfo{
					Version: &builder.Version{
						Version: *semver.MustParse(lifecycleVersion),
					},
				},
				APIs: builder.LifecycleAPIs{
					Buildpack: builder.APIVersions{
						Supported: builder.APISet{api.MustParse("0.2"), api.MustParse("0.3"), api.MustParse("0.4")},
					},
					Platform: builder.APIVersions{
						Supported: builder.APISet{api.MustParse("0.3"), api.MustParse("0.4")},
					},
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
		osImageCreator,
	)
}

type executeFailsLifecycle struct {
	Opts build.LifecycleOptions
}

func (f *executeFailsLifecycle) Execute(_ context.Context, opts build.LifecycleOptions) error {
	f.Opts = opts
	return errors.New("")
}
