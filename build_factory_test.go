package pack_test

import (
	"bytes"
	"context"
	"io/ioutil"
	"math/rand"
	"os"
	"testing"
	"time"

	"github.com/fatih/color"

	"github.com/buildpack/pack/cache"
	"github.com/buildpack/pack/config"
	"github.com/buildpack/pack/logging"

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
		cancelFunc         context.CancelFunc
	)

	it.Before(func() {
		var err error
		mockController = gomock.NewController(t)
		ctx, cancelFunc = context.WithCancel(context.TODO())

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

				mockDockerCli.EXPECT().ContainerCreate(ctx, &container.Config{
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
				mockDockerCli.EXPECT().ContainerCreate(ctx, &container.Config{
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
						mockDockerCli.EXPECT().CopyToContainer(ctx, "container-id", "/", nil, dockertypes.CopyToContainerOptions{}).Return(errors.New("error copy"))
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
						mockDockerCli.EXPECT().CopyToContainer(ctx, "container-id", "/", nil, dockertypes.CopyToContainerOptions{}).Return(nil)
					})

					when("cannot retrieve the pack UID and GID", func() {
						it.Before(func() {
							mockDockerCli.EXPECT().ImageInspectWithRaw(ctx, defaultBuilderName).Return(dockertypes.ImageInspect{}, nil, errors.New("inspect image error"))
						})

						it("returns an error", func() {
							err := subject.Detect(ctx)
							h.AssertError(t, err, "get pack uid gid: reading builder env variables: inspect image error")
						})
					})

					when("can retrieve pack UID and GID", func() {
						it.Before(func() {
							mockDockerCli.EXPECT().ImageInspectWithRaw(ctx, defaultBuilderName).Return(dockertypes.ImageInspect{
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
								mockDockerCli.EXPECT().ContainerCreate(ctx, &container.Config{
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
								mockDockerCli.EXPECT().ContainerCreate(ctx, &container.Config{
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
								mockDockerCli.EXPECT().RunContainer(ctx, "some-other-container-id", logger.VerboseWriter(), logger.VerboseErrorWriter()).Return(nil)
							})

							when("doesn't need to copy environment variables", func() {
								it.Before(func() {
									subject.EnvFile = map[string]string{}
								})

								when("fails to run the detect container", func() {
									it.Before(func() {
										mockDockerCli.EXPECT().RunContainer(
											ctx,
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
											ctx,
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
						mockDockerCli.EXPECT().CopyToContainer(ctx, "container-id", "/", nil, dockertypes.CopyToContainerOptions{}).Return(nil)
						mockFS.EXPECT().CreateTarReader("buildpack2", "/buildpacks/...", 0, 0).Return(nil, errChan)
						mockDockerCli.EXPECT().CopyToContainer(ctx, "container-id", "/", nil, dockertypes.CopyToContainerOptions{}).Return(nil)
					})

					when("copies the application to the container", func() {
						it.Before(func() {
							errChan := make(chan error, 1)
							errChan <- nil
							mockFS.EXPECT().CreateTarReader("acceptance/testdata/node_app", "/workspace/app", 0, 0).Return(nil, errChan)
							mockDockerCli.EXPECT().CopyToContainer(ctx, "container-id", "/", nil, dockertypes.CopyToContainerOptions{}).Return(nil)
						})

						when("can retrieve pack UID and GID", func() {
							it.Before(func() {
								mockDockerCli.EXPECT().ImageInspectWithRaw(ctx, defaultBuilderName).Return(dockertypes.ImageInspect{
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
									mockDockerCli.EXPECT().ContainerCreate(ctx, &container.Config{
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
									mockDockerCli.EXPECT().RunContainer(ctx, "some-other-container-id", logger.VerboseWriter(), logger.VerboseErrorWriter()).Return(nil)
								})

								when("creates the toml file", func() {
									it.Before(func() {
										mockFS.EXPECT().CreateSingleFileTar("/buildpacks/order.toml", gomock.Any()).Return(nil, nil)
									})
									when("copies the toml file to the container", func() {
										it.Before(func() {
											mockDockerCli.EXPECT().CopyToContainer(ctx, "container-id", "/", nil, dockertypes.CopyToContainerOptions{}).Return(nil)
										})

										when("doesn't need to copy environment variables", func() {
											it.Before(func() {
												subject.EnvFile = map[string]string{}
											})

											when("fails to run the detect container", func() {
												it.Before(func() {
													mockDockerCli.EXPECT().RunContainer(
														ctx,
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
														ctx,
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

			when("the process is terminated", func() {
				it.Before(func() {
					errChan := make(chan error, 1)
					errChan <- nil
					mockFS.EXPECT().CreateTarReader("acceptance/testdata/node_app", "/workspace/app", 0, 0).Return(nil, errChan)
					mockCache.EXPECT().Volume().Return("some-volume-name").AnyTimes()
				})
				it("stops the running container and cleans up", func() {
					mockDockerCli.EXPECT().
						CopyToContainer(ctx, "container-id", "/", nil, dockertypes.CopyToContainerOptions{}).
						DoAndReturn(func(ctx context.Context, arg0, arg1, arg2, arg3 interface{}) error {
							select {
							case <-ctx.Done():
								return ctx.Err()
							}
						})

					mockDockerCli.EXPECT().
						ContainerRemove(gomock.Any(), "container-id", dockertypes.ContainerRemoveOptions{Force: true}).
						DoAndReturn(func(_ context.Context, containerID string, options dockertypes.ContainerRemoveOptions) error {
							h.AssertError(t, ctx.Err(), "context canceled")
							return nil
						})

					time.AfterFunc(time.Second*1, cancelFunc)

					err := subject.Detect(ctx)
					h.AssertContains(t, err.Error(), "context canceled")
				})
			})
		})

	}, spec.Parallel())

	// TODO: Missing Unit tests for the other lifecycle steps
}
