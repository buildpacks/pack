package pack_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"math/rand"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/fatih/color"

	"github.com/buildpack/pack/cache"
	"github.com/buildpack/pack/config"
	"github.com/buildpack/pack/logging"

	"github.com/buildpack/lifecycle"
	dockertypes "github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/golang/mock/gomock"
	"github.com/pkg/errors"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	"github.com/buildpack/pack"
	"github.com/buildpack/pack/docker"
	"github.com/buildpack/pack/fs"
	"github.com/buildpack/pack/mocks"
	h "github.com/buildpack/pack/testhelpers"
)

var registryPort string

func TestBuildFactory(t *testing.T) {
	color.NoColor = true
	rand.Seed(time.Now().UTC().UnixNano())

	registryPort = h.RunRegistry(t, true)
	defer h.StopRegistry(t)
	packHome, err := ioutil.TempDir("", "build-test-pack-home")
	h.AssertNil(t, err)
	defer os.RemoveAll(packHome)
	h.ConfigurePackHome(t, packHome, registryPort)
	defer h.CleanDefaultImages(t, registryPort)

	spec.Run(t, "build_factory", testBuildFactory, spec.Report(report.Terminal{}))
}

func testBuildFactory(t *testing.T, when spec.G, it spec.S) {
	var (
		subject            *pack.BuildConfig
		outBuf             bytes.Buffer
		errBuf             bytes.Buffer
		dockerCli          *docker.Client
		logger             *logging.Logger
		mockController     *gomock.Controller
		defaultBuilderName string
		ctx                context.Context
	)

	it.Before(func() {
		var err error
		mockController = gomock.NewController(t)
		ctx = context.TODO()

		logger = logging.NewLogger(&outBuf, &errBuf, true, false)
		dockerCli, err = docker.New()
		h.AssertNil(t, err)
		repoName := "pack.build." + h.RandString(10)
		buildCache, err := cache.New(repoName, dockerCli)
		defaultBuilderName = h.DefaultBuilderImage(t, registryPort)
		subject = &pack.BuildConfig{
			AppDir:   "acceptance/testdata/node_app",
			Builder:  defaultBuilderName,
			RunImage: h.DefaultRunImage(t, registryPort),
			RepoName: repoName,
			Publish:  false,
			Cache:    buildCache,
			Logger:   logger,
			FS:       &fs.FS{},
			Cli:      dockerCli,
		}
	})

	when("#BuildConfigFromFlags", func() {
		var (
			factory          *pack.BuildFactory
			mockImageFactory *mocks.MockImageFactory
			mockDocker       *mocks.MockDocker
			mockCache        *mocks.MockCache
		)

		it.Before(func() {
			mockImageFactory = mocks.NewMockImageFactory(mockController)
			mockDocker = mocks.NewMockDocker(mockController)
			mockCache = mocks.NewMockCache(mockController)

			factory = &pack.BuildFactory{
				ImageFactory: mockImageFactory,
				Config: &config.Config{
					DefaultBuilder: "some/builder",
				},
				Cli:    mockDocker,
				Logger: logger,
				Cache:  mockCache,
			}

			mockCache.EXPECT().Volume().AnyTimes()
		})

		it.After(func() {
			mockController.Finish()
		})

		it("defaults to daemon, default-builder, pulls builder and run images, selects run-image from builder", func() {
			mockBuilderImage := mocks.NewMockImage(mockController)
			mockBuilderImage.EXPECT().Label("io.buildpacks.stack.id").Return("some.stack.id", nil)
			mockBuilderImage.EXPECT().Label("io.buildpacks.pack.metadata").Return(`{"runImages": ["some/run"]}`, nil)
			mockImageFactory.EXPECT().NewLocal("some/builder", true).Return(mockBuilderImage, nil)

			mockRunImage := mocks.NewMockImage(mockController)
			mockRunImage.EXPECT().Label("io.buildpacks.stack.id").Return("some.stack.id", nil)
			mockRunImage.EXPECT().Found().Return(true, nil)
			mockImageFactory.EXPECT().NewLocal("some/run", true).Return(mockRunImage, nil)

			config, err := factory.BuildConfigFromFlags(&pack.BuildFlags{
				RepoName: "some/app",
				Builder:  "",
			})
			h.AssertNil(t, err)
			h.AssertEq(t, config.RunImage, "some/run")
			h.AssertEq(t, config.Builder, "some/builder")
		})

		it("respects builder from flags", func() {
			mockBuilderImage := mocks.NewMockImage(mockController)
			mockBuilderImage.EXPECT().Label("io.buildpacks.stack.id").Return("some.stack.id", nil)
			mockBuilderImage.EXPECT().Label("io.buildpacks.pack.metadata").Return(`{"runImages": ["some/run"]}`, nil)
			mockImageFactory.EXPECT().NewLocal("custom/builder", true).Return(mockBuilderImage, nil)

			mockRunImage := mocks.NewMockImage(mockController)
			mockRunImage.EXPECT().Label("io.buildpacks.stack.id").Return("some.stack.id", nil)
			mockRunImage.EXPECT().Found().Return(true, nil)
			mockImageFactory.EXPECT().NewLocal("some/run", true).Return(mockRunImage, nil)

			config, err := factory.BuildConfigFromFlags(&pack.BuildFlags{
				RepoName: "some/app",
				Builder:  "custom/builder",
			})
			h.AssertNil(t, err)
			h.AssertEq(t, config.RunImage, "some/run")
			h.AssertEq(t, config.Builder, "custom/builder")
		})

		it("doesn't pull builder or run images when --no-pull is passed", func() {
			mockBuilderImage := mocks.NewMockImage(mockController)
			mockBuilderImage.EXPECT().Label("io.buildpacks.stack.id").Return("some.stack.id", nil)
			mockBuilderImage.EXPECT().Label("io.buildpacks.pack.metadata").Return(`{"runImages": ["some/run"]}`, nil)
			mockImageFactory.EXPECT().NewLocal("custom/builder", false).Return(mockBuilderImage, nil)

			mockRunImage := mocks.NewMockImage(mockController)
			mockRunImage.EXPECT().Label("io.buildpacks.stack.id").Return("some.stack.id", nil)
			mockRunImage.EXPECT().Found().Return(true, nil)
			mockImageFactory.EXPECT().NewLocal("some/run", false).Return(mockRunImage, nil)

			config, err := factory.BuildConfigFromFlags(&pack.BuildFlags{
				NoPull:   true,
				RepoName: "some/app",
				Builder:  "custom/builder",
			})
			h.AssertNil(t, err)
			h.AssertEq(t, config.RunImage, "some/run")
			h.AssertEq(t, config.Builder, "custom/builder")
		})

		it("selects run images with matching registry", func() {
			mockBuilderImage := mocks.NewMockImage(mockController)
			mockBuilderImage.EXPECT().Label("io.buildpacks.stack.id").Return("some.stack.id", nil)
			mockBuilderImage.EXPECT().Label("io.buildpacks.pack.metadata").Return(`{"runImages": ["some/run", "registry.com/some/run"]}`, nil)
			mockImageFactory.EXPECT().NewLocal("some/builder", true).Return(mockBuilderImage, nil)

			mockRunImage := mocks.NewMockImage(mockController)
			mockRunImage.EXPECT().Label("io.buildpacks.stack.id").Return("some.stack.id", nil)
			mockRunImage.EXPECT().Found().Return(true, nil)
			mockImageFactory.EXPECT().NewLocal("registry.com/some/run", true).Return(mockRunImage, nil)

			config, err := factory.BuildConfigFromFlags(&pack.BuildFlags{
				RepoName: "registry.com/some/app",
				Builder:  "some/builder",
			})
			h.AssertNil(t, err)
			h.AssertEq(t, config.RunImage, "registry.com/some/run")
			h.AssertEq(t, config.Builder, "some/builder")
		})

		when("both builder and local override run images have a matching registry", func() {
			it.Before(func() {
				factory.Config.Builders = []config.Builder{
					{
						Image:     "some/builder",
						RunImages: []string{"registry.com/override/run"},
					},
				}

				mockBuilderImage := mocks.NewMockImage(mockController)
				mockBuilderImage.EXPECT().Label("io.buildpacks.stack.id").Return("some.stack.id", nil)
				mockBuilderImage.EXPECT().Label("io.buildpacks.pack.metadata").Return(`{"runImages": ["registry.com/default/run", "default/run"]}`, nil)
				mockImageFactory.EXPECT().NewLocal("some/builder", true).Return(mockBuilderImage, nil)

				mockRunImage := mocks.NewMockImage(mockController)
				mockRunImage.EXPECT().Label("io.buildpacks.stack.id").Return("some.stack.id", nil)
				mockRunImage.EXPECT().Found().Return(true, nil)
				mockImageFactory.EXPECT().NewLocal("registry.com/override/run", true).Return(mockRunImage, nil)
			})

			it("selects from local override run images first", func() {
				config, err := factory.BuildConfigFromFlags(&pack.BuildFlags{
					RepoName: "registry.com/some/app",
					Builder:  "some/builder",
				})
				h.AssertNil(t, err)
				h.AssertEq(t, config.RunImage, "registry.com/override/run")
				h.AssertEq(t, config.Builder, "some/builder")
			})
		})

		it("uses a remote run image when --publish is passed", func() {
			mockBuilderImage := mocks.NewMockImage(mockController)
			mockBuilderImage.EXPECT().Label("io.buildpacks.stack.id").Return("some.stack.id", nil)
			mockBuilderImage.EXPECT().Label("io.buildpacks.pack.metadata").Return(`{"runImages": ["some/run"]}`, nil)
			mockImageFactory.EXPECT().NewLocal("some/builder", true).Return(mockBuilderImage, nil)

			mockRunImage := mocks.NewMockImage(mockController)
			mockRunImage.EXPECT().Label("io.buildpacks.stack.id").Return("some.stack.id", nil)
			mockRunImage.EXPECT().Found().Return(true, nil)
			mockImageFactory.EXPECT().NewRemote("some/run").Return(mockRunImage, nil)

			config, err := factory.BuildConfigFromFlags(&pack.BuildFlags{
				RepoName: "some/app",
				Builder:  "some/builder",
				Publish:  true,
			})
			h.AssertNil(t, err)
			h.AssertEq(t, config.RunImage, "some/run")
			h.AssertEq(t, config.Builder, "some/builder")
		})

		it("allows run-image from flags if the stacks match", func() {
			mockBuilderImage := mocks.NewMockImage(mockController)
			mockBuilderImage.EXPECT().Label("io.buildpacks.stack.id").Return("some.stack.id", nil)
			mockImageFactory.EXPECT().NewLocal("some/builder", true).Return(mockBuilderImage, nil)

			mockRunImage := mocks.NewMockImage(mockController)
			mockRunImage.EXPECT().Label("io.buildpacks.stack.id").Return("some.stack.id", nil)
			mockRunImage.EXPECT().Found().Return(true, nil)
			mockImageFactory.EXPECT().NewRemote("override/run").Return(mockRunImage, nil)

			config, err := factory.BuildConfigFromFlags(&pack.BuildFlags{
				RepoName: "some/app",
				Builder:  "some/builder",
				RunImage: "override/run",
				Publish:  true,
			})
			h.AssertNil(t, err)
			h.AssertEq(t, config.RunImage, "override/run")
			h.AssertEq(t, config.Builder, "some/builder")
		})

		it("doesn't allow run-image from flags if the stacks are different", func() {
			mockBuilderImage := mocks.NewMockImage(mockController)
			mockBuilderImage.EXPECT().Label("io.buildpacks.stack.id").Return("some.stack.id", nil)
			mockImageFactory.EXPECT().NewLocal("some/builder", true).Return(mockBuilderImage, nil)

			mockRunImage := mocks.NewMockImage(mockController)
			mockRunImage.EXPECT().Label("io.buildpacks.stack.id").Return("other.stack.id", nil)
			mockRunImage.EXPECT().Found().Return(true, nil)
			mockImageFactory.EXPECT().NewRemote("override/run").Return(mockRunImage, nil)

			_, err := factory.BuildConfigFromFlags(&pack.BuildFlags{
				RepoName: "some/app",
				Builder:  "some/builder",
				RunImage: "override/run",
				Publish:  true,
			})
			h.AssertError(t, err, "invalid stack: stack 'other.stack.id' from run image 'override/run' does not match stack 'some.stack.id' from builder image 'some/builder'")
		})

		it("uses working dir if appDir is set to placeholder value", func() {
			mockBuilderImage := mocks.NewMockImage(mockController)
			mockBuilderImage.EXPECT().Label("io.buildpacks.stack.id").Return("some.stack.id", nil)
			mockBuilderImage.EXPECT().Label("io.buildpacks.pack.metadata").Return(`{"runImages": ["some/run"]}`, nil)
			mockImageFactory.EXPECT().NewLocal("some/builder", true).Return(mockBuilderImage, nil)

			mockRunImage := mocks.NewMockImage(mockController)
			mockRunImage.EXPECT().Label("io.buildpacks.stack.id").Return("some.stack.id", nil)
			mockRunImage.EXPECT().Found().Return(true, nil)
			mockImageFactory.EXPECT().NewRemote("some/run").Return(mockRunImage, nil)

			config, err := factory.BuildConfigFromFlags(&pack.BuildFlags{
				RepoName: "some/app",
				Builder:  "some/builder",
				Publish:  true,
				AppDir:   "",
			})
			h.AssertNil(t, err)
			h.AssertEq(t, config.RunImage, "some/run")
			h.AssertEq(t, config.Builder, "some/builder")
			h.AssertEq(t, config.AppDir, os.Getenv("PWD"))
		})

		it("returns an error when the builder stack label is missing", func() {
			mockBuilderImage := mocks.NewMockImage(mockController)
			mockBuilderImage.EXPECT().Label("io.buildpacks.stack.id").Return("", nil)
			mockImageFactory.EXPECT().NewLocal("some/builder", true).Return(mockBuilderImage, nil)

			_, err := factory.BuildConfigFromFlags(&pack.BuildFlags{
				RepoName: "some/app",
				Builder:  "some/builder",
			})
			h.AssertError(t, err, "invalid builder image 'some/builder': missing required label 'io.buildpacks.stack.id'")
		})

		it("returns an error when the builder stack label is empty", func() {
			mockBuilderImage := mocks.NewMockImage(mockController)
			mockBuilderImage.EXPECT().Label("io.buildpacks.stack.id").Return("", nil)
			mockImageFactory.EXPECT().NewLocal("some/builder", true).Return(mockBuilderImage, nil)

			_, err := factory.BuildConfigFromFlags(&pack.BuildFlags{
				RepoName: "some/app",
				Builder:  "some/builder",
			})
			h.AssertError(t, err, "invalid builder image 'some/builder': missing required label 'io.buildpacks.stack.id'")
		})

		it("returns an error when the builder metadata label is missing", func() {
			mockBuilderImage := mocks.NewMockImage(mockController)
			mockBuilderImage.EXPECT().Label("io.buildpacks.stack.id").Return("some.stack.id", nil)
			mockBuilderImage.EXPECT().Label("io.buildpacks.pack.metadata").Return("", nil)
			mockImageFactory.EXPECT().NewLocal("some/builder", true).Return(mockBuilderImage, nil)

			_, err := factory.BuildConfigFromFlags(&pack.BuildFlags{
				RepoName: "some/app",
				Builder:  "some/builder",
			})
			h.AssertError(t, err, "invalid builder image 'some/builder': missing required label 'io.buildpacks.pack.metadata' -- try recreating builder")
		})

		it("returns an error when the builder metadata label is unparsable", func() {
			mockBuilderImage := mocks.NewMockImage(mockController)
			mockBuilderImage.EXPECT().Label("io.buildpacks.stack.id").Return("some.stack.id", nil)
			mockBuilderImage.EXPECT().Label("io.buildpacks.pack.metadata").Return("junk", nil)
			mockImageFactory.EXPECT().NewLocal("some/builder", true).Return(mockBuilderImage, nil)

			_, err := factory.BuildConfigFromFlags(&pack.BuildFlags{
				RepoName: "some/app",
				Builder:  "some/builder",
			})
			h.AssertError(t, err, "invalid builder image metadata: invalid character 'j' looking for beginning of value")
		})

		it("returns an error if remote run image doesn't exist in remote on published builds", func() {
			mockBuilderImage := mocks.NewMockImage(mockController)
			mockBuilderImage.EXPECT().Label("io.buildpacks.stack.id").Return("some.stack.id", nil)
			mockImageFactory.EXPECT().NewLocal("some/builder", true).Return(mockBuilderImage, nil)

			mockRunImage := mocks.NewMockImage(mockController)
			mockRunImage.EXPECT().Found().Return(false, nil)
			mockImageFactory.EXPECT().NewRemote("some/run").Return(mockRunImage, nil)

			_, err := factory.BuildConfigFromFlags(&pack.BuildFlags{
				RepoName: "some/app",
				Builder:  "some/builder",
				RunImage: "some/run",
				Publish:  true,
			})
			h.AssertError(t, err, "remote run image 'some/run' does not exist")
		})

		it("returns an error if local run image doesn't exist locally on local builds", func() {
			mockBuilderImage := mocks.NewMockImage(mockController)
			mockBuilderImage.EXPECT().Label("io.buildpacks.stack.id").Return("some.stack.id", nil)
			mockImageFactory.EXPECT().NewLocal("some/builder", true).Return(mockBuilderImage, nil)

			mockRunImage := mocks.NewMockImage(mockController)
			mockRunImage.EXPECT().Found().Return(false, nil)
			mockImageFactory.EXPECT().NewLocal("some/run", gomock.Any()).Return(mockRunImage, nil)

			_, err := factory.BuildConfigFromFlags(&pack.BuildFlags{
				RepoName: "some/app",
				Builder:  "some/builder",
				RunImage: "some/run",
				Publish:  false,
			})
			h.AssertError(t, err, "local run image 'some/run' does not exist")
		})

		it("sets EnvFile", func() {
			mockBuilderImage := mocks.NewMockImage(mockController)
			mockBuilderImage.EXPECT().Label("io.buildpacks.stack.id").Return("some.stack.id", nil)
			mockBuilderImage.EXPECT().Label("io.buildpacks.pack.metadata").Return(`{"runImages": ["some/run"]}`, nil)
			mockImageFactory.EXPECT().NewLocal("some/builder", true).Return(mockBuilderImage, nil)

			mockRunImage := mocks.NewMockImage(mockController)
			mockRunImage.EXPECT().Label("io.buildpacks.stack.id").Return("some.stack.id", nil)
			mockRunImage.EXPECT().Found().Return(true, nil)
			mockImageFactory.EXPECT().NewLocal("some/run", true).Return(mockRunImage, nil)

			envFile, err := ioutil.TempFile("", "pack.build.envfile")
			h.AssertNil(t, err)
			defer os.Remove(envFile.Name())

			_, err = envFile.Write([]byte(`
VAR1=value1
VAR2=value2 with spaces	
PATH
				`))
			h.AssertNil(t, err)
			envFile.Close()

			config, err := factory.BuildConfigFromFlags(&pack.BuildFlags{
				RepoName: "some/app",
				Builder:  "some/builder",
				EnvFile:  envFile.Name(),
			})
			h.AssertNil(t, err)
			h.AssertEq(t, config.EnvFile, map[string]string{
				"VAR1": "value1",
				"VAR2": "value2 with spaces",
				"PATH": os.Getenv("PATH"),
			})
			h.AssertNotEq(t, os.Getenv("PATH"), "")
		})
	}, spec.Parallel())

	when("#Detect", func() {
		var (
			mockDockerCli *mocks.MockDocker
			mockCache     *mocks.MockCache
			mockFS        *mocks.MockFS
		)

		it.Before(func() {
			mockCache = mocks.NewMockCache(mockController)
			mockDockerCli = mocks.NewMockDocker(mockController)
			mockFS = mocks.NewMockFS(mockController)

			subject.Cache = mockCache
			subject.Cli = mockDockerCli
			subject.FS = mockFS
		})

		when("clear cache flag is set to true", func() {
			it.Before(func() {
				subject.ClearCache = true
			})

			when("when fails to clear the cache", func() {
				it.Before(func() {
					mockCache.EXPECT().Clear(ctx).Return(errors.New("something went wrong"))
				})

				it("returns error", func() {
					err := subject.Detect(ctx)
					h.AssertError(t, err, "clearing cache: something went wrong")
				})
			})
		})

		when("fails to create a container", func() {
			it.Before(func() {
				mockCache.EXPECT().Volume().Return("some-volume-name")

				mockDockerCli.EXPECT().ContainerCreate(context.TODO(), &container.Config{
					Image: defaultBuilderName,
					Cmd: []string{
						"/lifecycle/detector",
						"-buildpacks", "/buildpacks",
						"-order", "/buildpacks/order.toml",
						"-group", "/workspace/group.toml",
						"-plan", "/workspace/plan.toml",
					},
					Labels: map[string]string{"author": "pack"},
				},
					&container.HostConfig{
						Binds: []string{"some-volume-name:/workspace:"},
					}, nil, "").Return(container.ContainerCreateCreatedBody{}, errors.New("unable to create container"))
			})

			it("returns error", func() {
				err := subject.Detect(ctx)
				h.AssertError(t, err, "create detect container: unable to create container")
			})
		})

		when("creates a new container", func() {
			it.Before(func() {
				mockDockerCli.EXPECT().ContainerCreate(context.TODO(), &container.Config{
					Image: defaultBuilderName,
					Cmd: []string{
						"/lifecycle/detector",
						"-buildpacks", "/buildpacks",
						"-order", "/buildpacks/order.toml",
						"-group", "/workspace/group.toml",
						"-plan", "/workspace/plan.toml",
					},
					Labels: map[string]string{"author": "pack"},
				},
					&container.HostConfig{
						Binds: []string{"some-volume-name:/workspace:"},
					}, nil, "").Return(container.ContainerCreateCreatedBody{
					ID: "container-id",
				}, nil)

				mockDockerCli.EXPECT().ContainerRemove(context.TODO(), "container-id", dockertypes.ContainerRemoveOptions{Force: true})
			})

			when("no buildpacks are provided", func() {
				it.Before(func() {
					subject.Buildpacks = []string{}
					mockCache.EXPECT().Volume().Return("some-volume-name").AnyTimes()
				})

				when("fails to copy the application to the container", func() {
					it.Before(func() {
						errChan := make(chan error, 1)
						mockFS.EXPECT().CreateTarReader("acceptance/testdata/node_app", "/workspace/app", 0, 0).Return(nil, errChan)
						mockDockerCli.EXPECT().CopyToContainer(context.TODO(), "container-id", "/", nil, dockertypes.CopyToContainerOptions{}).Return(errors.New("error copy"))
					})
					it("returns an error", func() {
						err := subject.Detect(ctx)
						h.AssertError(t, err, "copy app to workspace volume: error copy")
					})
				})

				when("copies the application to the container", func() {
					it.Before(func() {
						errChan := make(chan error, 1)
						errChan <- nil
						mockFS.EXPECT().CreateTarReader("acceptance/testdata/node_app", "/workspace/app", 0, 0).Return(nil, errChan)
						mockDockerCli.EXPECT().CopyToContainer(context.TODO(), "container-id", "/", nil, dockertypes.CopyToContainerOptions{}).Return(nil)
					})

					when("cannot retrieve the pack UID and GID", func() {
						it.Before(func() {
							mockDockerCli.EXPECT().ImageInspectWithRaw(context.TODO(), defaultBuilderName).Return(dockertypes.ImageInspect{}, nil, errors.New("inspect image error"))
						})

						it("returns an error", func() {
							err := subject.Detect(ctx)
							h.AssertError(t, err, "get pack uid gid: reading builder env variables: inspect image error")
						})
					})

					when("can retrieve pack UID and GID", func() {
						it.Before(func() {
							mockDockerCli.EXPECT().ImageInspectWithRaw(context.TODO(), defaultBuilderName).Return(dockertypes.ImageInspect{
								Config: &container.Config{
									Env: []string{
										"PACK_USER_ID=0000000",
										"PACK_GROUP_ID=8888888",
									},
								},
							}, nil, nil)
						})

						when("unable to change owner of the app directory", func() {
							it.Before(func() {
								mockDockerCli.EXPECT().ContainerCreate(context.TODO(), &container.Config{
									Image: defaultBuilderName,
									Cmd: []string{
										"chown",
										"-R", "0:8888888",
										"/workspace/app",
									},
									User:   "root",
									Labels: map[string]string{"author": "pack"},
								},
									&container.HostConfig{
										Binds: []string{"some-volume-name:/workspace:"},
									}, nil, "").Return(container.ContainerCreateCreatedBody{}, errors.New("error chown"))
							})
							it("returns an error", func() {
								err := subject.Detect(ctx)
								h.AssertError(t, err, "chown app to workspace volume: error chown")
							})
						})

						when("changes the ownership of the app directory", func() {
							it.Before(func() {
								mockDockerCli.EXPECT().ContainerCreate(context.TODO(), &container.Config{
									Image: defaultBuilderName,
									Cmd: []string{
										"chown",
										"-R", "0:8888888",
										"/workspace/app",
									},
									User:   "root",
									Labels: map[string]string{"author": "pack"},
								},
									&container.HostConfig{
										Binds: []string{"some-volume-name:/workspace:"},
									}, nil, "").Return(container.ContainerCreateCreatedBody{
									ID: "some-other-container-id",
								}, nil)
								mockDockerCli.EXPECT().ContainerRemove(context.TODO(), "some-other-container-id", dockertypes.ContainerRemoveOptions{Force: true})
								mockDockerCli.EXPECT().RunContainer(context.TODO(), "some-other-container-id", logger.VerboseWriter(), logger.VerboseErrorWriter()).Return(nil)
							})

							when("doesn't need to copy environment variables", func() {
								it.Before(func() {
									subject.EnvFile = map[string]string{}
								})

								when("fails to run the detect container", func() {
									it.Before(func() {
										mockDockerCli.EXPECT().RunContainer(
											context.TODO(),
											"container-id",
											logger.VerboseWriter().WithPrefix("detector"),
											logger.VerboseErrorWriter().WithPrefix("detector")).Return(errors.New("fatal error"))
									})

									it("returns an error", func() {
										err := subject.Detect(ctx)
										h.AssertError(t, err, "run detect container: fatal error")
									})
								})

								when("runs the detect container successfuly", func() {
									it.Before(func() {
										mockDockerCli.EXPECT().RunContainer(
											context.TODO(),
											"container-id",
											logger.VerboseWriter().WithPrefix("detector"),
											logger.VerboseErrorWriter().WithPrefix("detector")).Return(nil)
									})

									it("returns no error", func() {
										err := subject.Detect(ctx)
										h.AssertNil(t, err)
									})
								})
							})
						})
					})
				})
			})

			when("buildpacks are provided", func() {
				it.Before(func() {
					subject.Buildpacks = []string{"buildpack1", "buildpack2"}
					mockCache.EXPECT().Volume().Return("some-volume-name").AnyTimes()
				})

				when("copies the buildpacks to the container", func() {
					it.Before(func() {
						errChan := make(chan error, 2)
						errChan <- nil
						errChan <- nil
						mockFS.EXPECT().CreateTarReader("buildpack1", "/buildpacks/...", 0, 0).Return(nil, errChan)
						mockDockerCli.EXPECT().CopyToContainer(context.TODO(), "container-id", "/", nil, dockertypes.CopyToContainerOptions{}).Return(nil)
						mockFS.EXPECT().CreateTarReader("buildpack2", "/buildpacks/...", 0, 0).Return(nil, errChan)
						mockDockerCli.EXPECT().CopyToContainer(context.TODO(), "container-id", "/", nil, dockertypes.CopyToContainerOptions{}).Return(nil)
					})

					when("copies the application to the container", func() {
						it.Before(func() {
							errChan := make(chan error, 1)
							errChan <- nil
							mockFS.EXPECT().CreateTarReader("acceptance/testdata/node_app", "/workspace/app", 0, 0).Return(nil, errChan)
							mockDockerCli.EXPECT().CopyToContainer(context.TODO(), "container-id", "/", nil, dockertypes.CopyToContainerOptions{}).Return(nil)
						})

						when("can retrieve pack UID and GID", func() {
							it.Before(func() {
								mockDockerCli.EXPECT().ImageInspectWithRaw(context.TODO(), defaultBuilderName).Return(dockertypes.ImageInspect{
									Config: &container.Config{
										Env: []string{
											"PACK_USER_ID=0000000",
											"PACK_GROUP_ID=8888888",
										},
									},
								}, nil, nil)
							})

							when("changes the ownership of the app directory", func() {
								it.Before(func() {
									mockDockerCli.EXPECT().ContainerCreate(context.TODO(), &container.Config{
										Image: defaultBuilderName,
										Cmd: []string{
											"chown",
											"-R", "0:8888888",
											"/workspace/app",
										},
										User:   "root",
										Labels: map[string]string{"author": "pack"},
									},
										&container.HostConfig{
											Binds: []string{"some-volume-name:/workspace:"},
										}, nil, "").Return(container.ContainerCreateCreatedBody{
										ID: "some-other-container-id",
									}, nil)
									mockDockerCli.EXPECT().ContainerRemove(context.TODO(), "some-other-container-id", dockertypes.ContainerRemoveOptions{Force: true})
									mockDockerCli.EXPECT().RunContainer(context.TODO(), "some-other-container-id", logger.VerboseWriter(), logger.VerboseErrorWriter()).Return(nil)
								})

								when("creates the toml file", func() {
									it.Before(func() {
										mockFS.EXPECT().CreateSingleFileTar("/buildpacks/order.toml", gomock.Any()).Return(nil, nil)
									})
									when("copies the toml file to the container", func() {
										it.Before(func() {
											mockDockerCli.EXPECT().CopyToContainer(context.TODO(), "container-id", "/", nil, dockertypes.CopyToContainerOptions{}).Return(nil)
										})

										when("doesn't need to copy environment variables", func() {
											it.Before(func() {
												subject.EnvFile = map[string]string{}
											})

											when("fails to run the detect container", func() {
												it.Before(func() {
													mockDockerCli.EXPECT().RunContainer(
														context.TODO(),
														"container-id",
														logger.VerboseWriter().WithPrefix("detector"),
														logger.VerboseErrorWriter().WithPrefix("detector")).Return(errors.New("fatal error"))
												})

												it("returns an error", func() {
													err := subject.Detect(ctx)
													h.AssertError(t, err, "run detect container: fatal error")
												})
											})

											when("runs the detect container successfuly", func() {
												it.Before(func() {
													mockDockerCli.EXPECT().RunContainer(
														context.TODO(),
														"container-id",
														logger.VerboseWriter().WithPrefix("detector"),
														logger.VerboseErrorWriter().WithPrefix("detector")).Return(nil)
												})

												it("returns no error", func() {
													err := subject.Detect(ctx)
													h.AssertNil(t, err)
												})
											})
										})
									})
								})
							})
						})
					})
				})

			})
		})

	}, spec.Parallel())

	when("#Analyze", func() {
		it.Before(func() {
			var err error
			mockController = gomock.NewController(t)

			logger = logging.NewLogger(&outBuf, &errBuf, true, false)
			dockerCli, err = docker.New()
			h.AssertNil(t, err)
			repoName := "pack.build." + h.RandString(10)
			buildCache, err := cache.New(repoName, dockerCli)
			defaultBuilderName = h.DefaultBuilderImage(t, registryPort)
			subject = &pack.BuildConfig{
				AppDir:   "acceptance/testdata/node_app",
				Builder:  defaultBuilderName,
				RunImage: h.DefaultRunImage(t, registryPort),
				RepoName: repoName,
				Publish:  false,
				Cache:    buildCache,
				Logger:   logger,
				FS:       &fs.FS{},
				Cli:      dockerCli,
			}

			tmpDir, err := ioutil.TempDir("", "pack.build.analyze.")
			h.AssertNil(t, err)
			defer os.RemoveAll(tmpDir)
			h.AssertNil(t, ioutil.WriteFile(filepath.Join(tmpDir, "group.toml"), []byte(`[[buildpacks]]
			  id = "io.buildpacks.samples.nodejs"
			  version = "0.0.1"
			`), 0666))

			h.CopyWorkspaceToDocker(t, tmpDir, subject.Cache.Volume())
		})

		it.After(func() {
			for _, volName := range []string{subject.Cache.Volume(), subject.Cache.Volume()} {
				dockerCli.VolumeRemove(context.TODO(), volName, true)
			}
		})

		when("no previous image exists", func() {
			when("publish", func() {
				it.Before(func() {
					subject.RepoName = "localhost:" + registryPort + "/" + subject.RepoName
					subject.Publish = true
				})

				it("succeeds and does nothing", func() {
					err := subject.Analyze(ctx)
					h.AssertNil(t, err)
				})
			})

			when("succeeds and does nothing", func() {
				it.Before(func() { subject.Publish = false })
				it("succeeds and does nothing", func() {
					err := subject.Analyze(ctx)
					h.AssertNil(t, err)
				})
			})
		})

		when("previous image exists", func() {
			var dockerFile string
			it.Before(func() {
				dockerFile = fmt.Sprintf(`
					FROM busybox
					LABEL io.buildpacks.lifecycle.metadata='{"buildpacks":[{"key":"io.buildpacks.samples.nodejs","layers":{"node_modules":{"launch": true, "sha":"sha256:99311ec03d790adf46d35cd9219ed80a7d9a4b97f761247c02c77e7158a041d5","data":{"lock_checksum":"eb04ed1b461f1812f0f4233ef997cdb5"}}}}]}'
					LABEL repo_name_for_randomisation=%s
				`, subject.RepoName)
			})

			when("publish", func() {
				it.Before(func() {
					subject.Publish = true
					subject.RepoName = h.CreateImageOnRemote(t, dockerCli, registryPort, subject.RepoName, dockerFile)
				})

				it("places files in workspace and sets owner to pack", func() {
					h.AssertNil(t, subject.Analyze(ctx))

					txt := h.ReadFromDocker(t, subject.Cache.Volume(), "/workspace/io.buildpacks.samples.nodejs/node_modules.toml")

					h.AssertEq(t, txt, `build = false
launch = true
cache = false

[metadata]
  lock_checksum = "eb04ed1b461f1812f0f4233ef997cdb5"
`)
					hdr := h.StatFromDocker(t, subject.Cache.Volume(), "/workspace/io.buildpacks.samples.nodejs/node_modules.toml")
					h.AssertEq(t, hdr.Uid, 1000)
					h.AssertEq(t, hdr.Gid, 1000)
				})
			})

			when("daemon", func() {
				it.Before(func() {
					subject.Publish = false

					h.CreateImageOnLocal(t, dockerCli, subject.RepoName, dockerFile)
				})

				it.After(func() {
					h.AssertNil(t, h.DockerRmi(dockerCli, subject.RepoName))
				})

				it("places files in workspace and sets owner to pack", func() {
					err := subject.Analyze(ctx)
					h.AssertNil(t, err)

					txt := h.ReadFromDocker(t, subject.Cache.Volume(), "/workspace/io.buildpacks.samples.nodejs/node_modules.toml")
					h.AssertEq(t, txt, `build = false
launch = true
cache = false

[metadata]
  lock_checksum = "eb04ed1b461f1812f0f4233ef997cdb5"
`)
					hdr := h.StatFromDocker(t, subject.Cache.Volume(), "/workspace/io.buildpacks.samples.nodejs/node_modules.toml")
					h.AssertEq(t, hdr.Uid, 1000)
					h.AssertEq(t, hdr.Gid, 1000)
				})
			})
		})
	}, spec.Sequential())

	when("#Build", func() {
		it.Before(func() {
			var err error

			logger = logging.NewLogger(&outBuf, &errBuf, true, false)
			dockerCli, err = docker.New()
			h.AssertNil(t, err)
			repoName := "pack.build." + h.RandString(10)
			buildCache, err := cache.New(repoName, dockerCli)
			defaultBuilderName = h.DefaultBuilderImage(t, registryPort)
			subject = &pack.BuildConfig{
				AppDir:   "acceptance/testdata/node_app",
				Builder:  defaultBuilderName,
				RunImage: h.DefaultRunImage(t, registryPort),
				RepoName: repoName,
				Publish:  false,
				Cache:    buildCache,
				Logger:   logger,
				FS:       &fs.FS{},
				Cli:      dockerCli,
			}
		})
		it.After(func() {
			for _, volName := range []string{subject.Cache.Volume(), subject.Cache.Volume()} {
				dockerCli.VolumeRemove(context.TODO(), volName, true)
			}
		})

		when("buildpacks are specified", func() {
			when("directory buildpack", func() {
				var bpDir string
				it.Before(func() {
					var err error
					bpDir, err = ioutil.TempDir("", "pack.build.bpdir.")
					h.AssertNil(t, err)
					h.AssertNil(t, ioutil.WriteFile(filepath.Join(bpDir, "buildpack.toml"), []byte(`
					[buildpack]
					id = "com.example.mybuildpack"
					version = "1.2.3"
					name = "My Sample Buildpack"

					[[stacks]]
					id = "io.buildpacks.stacks.bionic"
					`), 0666))
					h.AssertNil(t, os.MkdirAll(filepath.Join(bpDir, "bin"), 0777))
					h.AssertNil(t, ioutil.WriteFile(filepath.Join(bpDir, "bin", "detect"), []byte(`#!/usr/bin/env bash
					exit 0
					`), 0777))
					h.AssertNil(t, ioutil.WriteFile(filepath.Join(bpDir, "bin", "build"), []byte(`#!/usr/bin/env bash
					echo "BUILD OUTPUT FROM MY SAMPLE BUILDPACK"
					exit 0
					`), 0777))
				})
				it.After(func() {
					os.RemoveAll(bpDir)
				})

				it("runs the buildpacks bin/build", func() {
					if runtime.GOOS == "windows" {
						t.Skip("directory buildpacks are not implemented on windows")
					}
					subject.Buildpacks = []string{bpDir}

					h.AssertNil(t, subject.Detect(ctx))
					h.AssertNil(t, subject.Build(ctx))

					h.AssertContains(t, outBuf.String(), "BUILD OUTPUT FROM MY SAMPLE BUILDPACK")
				})
			})
			when("id@version buildpack", func() {
				it("runs the buildpacks bin/build", func() {
					subject.Buildpacks = []string{"io.buildpacks.samples.nodejs@latest"}

					h.AssertNil(t, subject.Detect(ctx))
					h.AssertNil(t, subject.Build(ctx))

					h.AssertContains(t, outBuf.String(), "Sample Node.js Buildpack: pass")
				})
			})
		})

		when("EnvFile is specified", func() {
			it("sets specified env variables in /platform/env/...", func() {
				if runtime.GOOS == "windows" {
					t.Skip("directory buildpacks are not implemented on windows")
				}
				subject.EnvFile = map[string]string{
					"VAR1": "value1",
					"VAR2": "value2 with spaces",
				}
				subject.Buildpacks = []string{"acceptance/testdata/mock_buildpacks/printenv"}
				h.AssertNil(t, subject.Detect(ctx))
				h.AssertNil(t, subject.Build(ctx))
				h.AssertContains(t, outBuf.String(), "BUILD: VAR1 is value1;")
				h.AssertContains(t, outBuf.String(), "BUILD: VAR2 is value2 with spaces;")
			})
		})
	}, spec.Sequential())

	when("#Export", func() {
		var (
			runSHA         string
			runTopLayer    string
			setupLayersDir func()
		)
		it.Before(func() {
			var err error

			logger = logging.NewLogger(&outBuf, &errBuf, true, false)
			dockerCli, err = docker.New()
			h.AssertNil(t, err)
			repoName := "pack.build." + h.RandString(10)
			buildCache, err := cache.New(repoName, dockerCli)
			defaultBuilderName = h.DefaultBuilderImage(t, registryPort)
			subject = &pack.BuildConfig{
				AppDir:   "acceptance/testdata/node_app",
				Builder:  defaultBuilderName,
				RunImage: h.DefaultRunImage(t, registryPort),
				RepoName: repoName,
				Publish:  false,
				Cache:    buildCache,
				Logger:   logger,
				FS:       &fs.FS{},
				Cli:      dockerCli,
			}

			tmpDir, err := ioutil.TempDir("", "pack.build.export.")
			h.AssertNil(t, err)
			defer os.RemoveAll(tmpDir)
			setupLayersDir = func() {
				files := map[string]string{
					"group.toml":           "[[buildpacks]]\n" + `id = "io.buildpacks.samples.nodejs"` + "\n" + `version = "0.0.1"`,
					"app/file.txt":         "some text",
					"config/metadata.toml": "stuff = \"text\"",
					"io.buildpacks.samples.nodejs/mylayer.toml":     "launch = true\n[metadata]\n  key = \"myval\"",
					"io.buildpacks.samples.nodejs/mylayer/file.txt": "content",
					"io.buildpacks.samples.nodejs/other.toml":       "launch = true",
					"io.buildpacks.samples.nodejs/other/file.txt":   "something",
				}
				for name, txt := range files {
					h.AssertNil(t, os.MkdirAll(filepath.Dir(filepath.Join(tmpDir, name)), 0777))
					h.AssertNil(t, ioutil.WriteFile(filepath.Join(tmpDir, name), []byte(txt), 0666))
				}
				h.CopyWorkspaceToDocker(t, tmpDir, subject.Cache.Volume())
			}
			setupLayersDir()

			runSHA = imageSHA(t, dockerCli, subject.RunImage)
			runTopLayer = topLayer(t, dockerCli, subject.RunImage)
		})

		it.After(func() {
			for _, volName := range []string{subject.Cache.Volume(), subject.Cache.Volume()} {
				dockerCli.VolumeRemove(context.TODO(), volName, true)
			}
		})

		when("publish", func() {
			var oldRepoName string
			it.Before(func() {
				oldRepoName = subject.RepoName

				subject.RepoName = "localhost:" + registryPort + "/" + oldRepoName
				subject.Publish = true
			})

			it.After(func() {
				if t.Failed() {
					t.Log("OUTPUT:", outBuf.String())
				}
			})

			it("creates the image on the registry", func() {
				h.AssertNil(t, subject.Export(ctx))
				images := h.HttpGet(t, "http://localhost:"+registryPort+"/v2/_catalog")
				h.AssertContains(t, images, oldRepoName)
			})

			it("puts the files on the image", func() {
				h.AssertNil(t, subject.Export(ctx))

				h.AssertNil(t, h.PullImage(dockerCli, subject.RepoName))
				defer h.DockerRmi(dockerCli, subject.RepoName)
				txt, err := h.CopySingleFileFromImage(dockerCli, subject.RepoName, "workspace/app/file.txt")
				h.AssertNil(t, err)
				h.AssertEq(t, string(txt), "some text")

				txt, err = h.CopySingleFileFromImage(dockerCli, subject.RepoName, "workspace/io.buildpacks.samples.nodejs/mylayer/file.txt")
				h.AssertNil(t, err)
				h.AssertEq(t, string(txt), "content")
			})

			it("sets the metadata on the image", func() {
				h.AssertNil(t, subject.Export(ctx))

				h.AssertNil(t, h.PullImage(dockerCli, subject.RepoName))
				defer h.DockerRmi(dockerCli, subject.RepoName)
				var metadata lifecycle.AppImageMetadata
				metadataJSON := imageLabel(t, dockerCli, subject.RepoName, "io.buildpacks.lifecycle.metadata")
				t.Log(metadataJSON)
				h.AssertNil(t, json.Unmarshal([]byte(metadataJSON), &metadata))

				h.AssertEq(t, metadata.RunImage.SHA, runSHA)
				h.AssertEq(t, metadata.RunImage.TopLayer, runTopLayer)
				h.AssertContains(t, metadata.App.SHA, "sha256:")
				h.AssertContains(t, metadata.Config.SHA, "sha256:")
				h.AssertEq(t, len(metadata.Buildpacks), 1)
				h.AssertContains(t, metadata.Buildpacks[0].Layers["mylayer"].SHA, "sha256:")
				h.AssertEq(t, metadata.Buildpacks[0].Layers["mylayer"].Data, map[string]interface{}{"key": "myval"})
				h.AssertContains(t, metadata.Buildpacks[0].Layers["other"].SHA, "sha256:")
			})
		})

		when("daemon", func() {
			it.Before(func() { subject.Publish = false })

			it.After(func() {
				if t.Failed() {
					t.Log("OUTPUT:", outBuf.String())
				}
				h.AssertNil(t, h.DockerRmi(dockerCli, subject.RepoName))
			})

			it("creates the image on the daemon", func() {
				h.AssertNil(t, subject.Export(ctx))
				images := imageList(t, dockerCli)
				h.AssertSliceContains(t, images, subject.RepoName+":latest")
			})
			it("puts the files on the image", func() {
				h.AssertNil(t, subject.Export(ctx))

				txt, err := h.CopySingleFileFromImage(dockerCli, subject.RepoName, "workspace/app/file.txt")
				h.AssertNil(t, err)
				h.AssertEq(t, string(txt), "some text")

				txt, err = h.CopySingleFileFromImage(dockerCli, subject.RepoName, "workspace/io.buildpacks.samples.nodejs/mylayer/file.txt")
				h.AssertNil(t, err)
				h.AssertEq(t, string(txt), "content")
			})
			it("sets the metadata on the image", func() {
				h.AssertNil(t, subject.Export(ctx))

				var metadata lifecycle.AppImageMetadata
				metadataJSON := imageLabel(t, dockerCli, subject.RepoName, "io.buildpacks.lifecycle.metadata")
				h.AssertNil(t, json.Unmarshal([]byte(metadataJSON), &metadata))

				h.AssertEq(t, metadata.RunImage.SHA, runSHA)
				h.AssertEq(t, metadata.RunImage.TopLayer, runTopLayer)
				h.AssertContains(t, metadata.App.SHA, "sha256:")
				h.AssertContains(t, metadata.Config.SHA, "sha256:")
				h.AssertEq(t, len(metadata.Buildpacks), 1)
				h.AssertContains(t, metadata.Buildpacks[0].Layers["mylayer"].SHA, "sha256:")
				h.AssertEq(t, metadata.Buildpacks[0].Layers["mylayer"].Data, map[string]interface{}{"key": "myval"})
				h.AssertContains(t, metadata.Buildpacks[0].Layers["other"].SHA, "sha256:")
			})

			when("PACK_USER_ID and PACK_GROUP_ID are set on builder", func() {
				it.Before(func() {
					subject.Builder = "packs/samples-" + h.RandString(8)
					h.CreateImageOnLocal(t, dockerCli, subject.Builder, fmt.Sprintf(`
						FROM %s
						ENV PACK_USER_ID 1234
						ENV PACK_GROUP_ID 5678
						LABEL repo_name_for_randomisation=%s
					`, h.DefaultBuilderImage(t, registryPort), subject.Builder))
				})

				it.After(func() {
					h.AssertNil(t, h.DockerRmi(dockerCli, subject.Builder))
				})

				it("sets owner of layer files to PACK_USER_ID:PACK_GROUP_ID", func() {
					h.AssertNil(t, subject.Export(ctx))
					txt := h.RunInImage(t, dockerCli, nil, subject.RepoName, "ls", "-la", "/workspace/app/file.txt")
					h.AssertContains(t, txt, " 1234 5678 ")
				})
			})

			when("previous image exists", func() {
				it.Before(func() {
					t.Log("create image and h.Assert add new layer")
					h.AssertNil(t, subject.Export(ctx))
					setupLayersDir()
				})

				it("reuses images from previous layers", func() {
					origImageID := h.ImageID(t, subject.RepoName)
					defer func() { h.AssertNil(t, h.DockerRmi(dockerCli, origImageID)) }()

					txt, err := h.CopySingleFileFromImage(dockerCli, subject.RepoName, "workspace/io.buildpacks.samples.nodejs/mylayer/file.txt")
					h.AssertNil(t, err)
					h.AssertEq(t, txt, "content")

					t.Log("setup workspace to reuse layer")
					outBuf.Reset()
					h.RunInImage(t, dockerCli,
						[]string{subject.Cache.Volume() + ":/workspace"},
						h.DefaultBuilderImage(t, registryPort),
						"rm", "-rf", "/workspace/io.buildpacks.samples.nodejs/mylayer",
					)

					t.Log("recreate image and h.Assert copying layer from previous image")
					h.AssertNil(t, subject.Export(ctx))
					txt, err = h.CopySingleFileFromImage(dockerCli, subject.RepoName, "workspace/io.buildpacks.samples.nodejs/mylayer/file.txt")
					h.AssertNil(t, err)
					h.AssertEq(t, txt, "content")
				})
			})
		})
	}, spec.Sequential())
}

func imageSHA(t *testing.T, dockerCli *docker.Client, repoName string) string {
	t.Helper()
	inspect, _, err := dockerCli.ImageInspectWithRaw(context.Background(), repoName)
	h.AssertNil(t, err)
	sha := strings.Split(inspect.RepoDigests[0], "@")[1]
	return sha
}

func topLayer(t *testing.T, dockerCli *docker.Client, repoName string) string {
	t.Helper()
	inspect, _, err := dockerCli.ImageInspectWithRaw(context.Background(), repoName)
	h.AssertNil(t, err)
	layers := inspect.RootFS.Layers
	return layers[len(layers)-1]
}

func imageLabel(t *testing.T, dockerCli *docker.Client, repoName, labelName string) string {
	t.Helper()
	inspect, _, err := dockerCli.ImageInspectWithRaw(context.Background(), repoName)
	h.AssertNil(t, err)
	return inspect.Config.Labels[labelName]
}

func imageList(t *testing.T, dockerCli *docker.Client) []string {
	t.Helper()
	var out []string
	list, err := dockerCli.ImageList(context.Background(), dockertypes.ImageListOptions{})
	h.AssertNil(t, err)
	for _, s := range list {
		out = append(out, s.RepoTags...)
	}
	return out
}
