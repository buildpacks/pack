package pack_test

import (
	"bytes"
	"context"
	"crypto/md5"
	"fmt"
	"io"
	"math/rand"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/go-connections/nat"
	"github.com/fatih/color"
	"github.com/golang/mock/gomock"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	"github.com/buildpack/pack"
	"github.com/buildpack/pack/config"
	"github.com/buildpack/pack/fs"
	"github.com/buildpack/pack/logging"
	"github.com/buildpack/pack/mocks"
	h "github.com/buildpack/pack/testhelpers"
)

func TestRun(t *testing.T) {
	color.NoColor = true
	rand.Seed(time.Now().UTC().UnixNano())
	spec.Run(t, "run", testRun, spec.Parallel(), spec.Report(report.Terminal{}))
}

func testRun(t *testing.T, when spec.G, it spec.S) {
	var (
		outBuf         bytes.Buffer
		errBuf         bytes.Buffer
		logger         *logging.Logger
		mockController *gomock.Controller
		mockBuild      *mocks.MockBuildRunner
		mockDocker     *mocks.MockDocker
		ctx            context.Context
		cancel         context.CancelFunc
	)

	it.Before(func() {
		mockController = gomock.NewController(t)
		mockBuild = mocks.NewMockBuildRunner(mockController)
		mockDocker = mocks.NewMockDocker(mockController)
		logger = logging.NewLogger(&outBuf, &errBuf, true, false)
		ctx, cancel = context.WithTimeout(context.TODO(), time.Minute*2)
	})

	it.After(func() {
		mockController.Finish()
	})

	when("#RunConfigFromFlags", func() {
		var (
			mockController   *gomock.Controller
			factory          *pack.BuildFactory
			mockImageFactory *mocks.MockImageFactory
			mockCache        *mocks.MockCache
		)

		it.Before(func() {
			mockController = gomock.NewController(t)
			mockImageFactory = mocks.NewMockImageFactory(mockController)
			mockCache = mocks.NewMockCache(mockController)
			factory = &pack.BuildFactory{
				Logger:       logger,
				FS:           &fs.FS{},
				ImageFactory: mockImageFactory,
				Cache:        mockCache,
				Config: &config.Config{
					Stacks: []config.Stack{
						{
							ID:        "some.stack.id",
							RunImages: []string{"some/run", "registry.com/some/run"},
						},
					},
				},
			}

			mockCache.EXPECT().Image().Return("some-volume").AnyTimes()
		})

		it.After(func() {
			mockController.Finish()
		})

		it("creates args RunConfig derived from args BuildConfig", func() {
			mockBuilderImage := mocks.NewMockImage(mockController)
			mockImageFactory.EXPECT().NewLocal("some/builder", true).Return(mockBuilderImage, nil)

			mockRunImage := mocks.NewMockImage(mockController)
			mockRunImage.EXPECT().Found().Return(true, nil)
			mockImageFactory.EXPECT().NewLocal("some/run", true).Return(mockRunImage, nil)

			run, err := factory.RunConfigFromFlags(&pack.RunFlags{
				BuildFlags: pack.BuildFlags{
					AppDir:   "acceptance/testdata/node_app",
					Builder:  "some/builder",
					RunImage: "some/run",
				},
				Ports: []string{"1370"},
			})
			h.AssertNil(t, err)

			absAppDir, _ := filepath.Abs("acceptance/testdata/node_app")
			absAppDirMd5 := fmt.Sprintf("pack.local/run/%x", md5.Sum([]byte(absAppDir)))
			h.AssertEq(t, run.RepoName, absAppDirMd5)
			h.AssertEq(t, run.Ports, []string{"1370"})

			build, ok := run.Build.(*pack.BuildConfig)
			h.AssertEq(t, ok, true)
			for _, field := range []string{
				"RepoName",
				"Cli",
				"Logger",
			} {
				h.AssertSameInstance(
					t,
					reflect.Indirect(reflect.ValueOf(run)).FieldByName(field).Interface(),
					reflect.Indirect(reflect.ValueOf(build)).FieldByName(field).Interface(),
				)
			}
		})

	})

	when("#Run", func() {
		var (
			subject *pack.RunConfig
			ctr     container.ContainerCreateCreatedBody
		)

		it.Before(func() {
			subject = &pack.RunConfig{
				Build:    mockBuild,
				RepoName: "pack.local/run/346ffb210a2c6d138c8d058d6d4025a0",
				Ports:    []string{"1370"},
				Cli:      mockDocker,
				Logger:   logger,
			}
			ctr = container.ContainerCreateCreatedBody{
				ID: "29aef5a011dd",
			}
		})

		it("builds an image and runs it", func() {
			mockBuild.EXPECT().Run(ctx).Return(nil)

			exposedPorts, portBindings, _ := nat.ParsePortSpecs([]string{"127.0.0.1:1370:1370/tcp"})
			mockDocker.EXPECT().ContainerCreate(gomock.Any(), &container.Config{
				Image:        subject.RepoName,
				AttachStdout: true,
				AttachStderr: true,
				ExposedPorts: exposedPorts,
				Labels:       map[string]string{"author": "pack"},
			}, &container.HostConfig{
				AutoRemove:   true,
				PortBindings: portBindings,
			}, nil, "").Return(ctr, nil)

			mockDocker.EXPECT().ContainerRemove(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).Times(0)
			mockDocker.EXPECT().RunContainer(gomock.Any(), ctr.ID, gomock.Any(), gomock.Any()).Return(nil)
			mockDocker.EXPECT().ContainerRemove(gomock.Any(), ctr.ID, types.ContainerRemoveOptions{Force: true})

			err := subject.Run(ctx)
			h.AssertNil(t, err)

			h.AssertContains(t, outBuf.String(), "Starting container listening at http://localhost:1370/")
		})

		when("the build fails", func() {
			it("exits without running", func() {
				expected := fmt.Errorf("build error")
				mockBuild.EXPECT().Run(ctx).Return(expected)

				mockDocker.EXPECT().ContainerCreate(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Times(0)
				mockDocker.EXPECT().ContainerRemove(gomock.Any(), gomock.Any(), gomock.Any()).Times(0)
				mockDocker.EXPECT().RunContainer(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Times(0)

				err := subject.Run(ctx)
				h.AssertSameInstance(t, err, expected)
			})
		})

		when("the process is terminated", func() {
			it("stops the running container and cleans up", func() {
				mockBuild.EXPECT().Run(ctx).Return(nil)
				mockDocker.EXPECT().ContainerCreate(gomock.Any(), gomock.Any(), gomock.Any(), nil, "").Return(ctr, nil)

				mockDocker.EXPECT().
					RunContainer(gomock.Any(), ctr.ID, gomock.Any(), gomock.Any()).
					DoAndReturn(func(ctx context.Context, id string, stdout io.Writer, stderr io.Writer) error {
						select {
						case <-ctx.Done():
							return nil
						}
					})
				mockDocker.EXPECT().
					ContainerRemove(gomock.Any(), ctr.ID, types.ContainerRemoveOptions{Force: true}).
					DoAndReturn(func(_ context.Context, containerID string, options types.ContainerRemoveOptions) error {
						h.AssertError(t, ctx.Err(), "context canceled")
						return nil
					})

				time.AfterFunc(time.Second*1, cancel)

				err := subject.Run(ctx)
				h.AssertNil(t, err)
			})
		})

		when("the port is not specified", func() {
			it.Before(func() {
				subject.Ports = nil
			})

			it("gets exposed ports from the built image", func() {
				mockBuild.EXPECT().Run(ctx).Return(nil)

				exposedPorts, portBindings, _ := nat.ParsePortSpecs([]string{
					"127.0.0.1:8080:8080/tcp",
				})
				mockDocker.EXPECT().ImageInspectWithRaw(gomock.Any(), subject.RepoName).Return(types.ImageInspect{
					Config: &container.Config{
						ExposedPorts: exposedPorts,
					},
				}, []byte{}, nil)

				mockDocker.EXPECT().ContainerCreate(gomock.Any(), &container.Config{
					Image:        subject.RepoName,
					AttachStdout: true,
					AttachStderr: true,
					ExposedPorts: exposedPorts,
					Labels:       map[string]string{"author": "pack"},
				}, &container.HostConfig{
					AutoRemove:   true,
					PortBindings: portBindings,
				}, nil, "").Return(ctr, nil)

				mockDocker.EXPECT().RunContainer(gomock.Any(), ctr.ID, gomock.Any(), gomock.Any()).Return(nil)
				mockDocker.EXPECT().ContainerRemove(gomock.Any(), ctr.ID, types.ContainerRemoveOptions{Force: true})

				err := subject.Run(ctx)
				h.AssertNil(t, err)
			})
		})
		when("custom ports bindings are defined", func() {
			it("binds simple ports from localhost to the container on the same port", func() {
				mockBuild.EXPECT().Run(ctx).Return(nil)

				subject.Ports = []string{"1370"}
				exposedPorts, portBindings, _ := nat.ParsePortSpecs([]string{
					"127.0.0.1:1370:1370/tcp",
				})
				mockDocker.EXPECT().ContainerCreate(gomock.Any(), &container.Config{
					Image:        subject.RepoName,
					AttachStdout: true,
					AttachStderr: true,
					ExposedPorts: exposedPorts,
					Labels:       map[string]string{"author": "pack"},
				}, &container.HostConfig{
					AutoRemove:   true,
					PortBindings: portBindings,
				}, nil, "").Return(ctr, nil)

				mockDocker.EXPECT().RunContainer(gomock.Any(), ctr.ID, gomock.Any(), gomock.Any()).Return(nil)
				mockDocker.EXPECT().ContainerRemove(gomock.Any(), ctr.ID, types.ContainerRemoveOptions{Force: true})

				err := subject.Run(ctx)
				h.AssertNil(t, err)
			})
			it("binds each port to the container", func() {
				mockBuild.EXPECT().Run(ctx).Return(nil)

				subject.Ports = []string{
					"0.0.0.0:8080:8080/tcp",
					"0.0.0.0:8443:8443/tcp",
				}
				exposedPorts, portBindings, _ := nat.ParsePortSpecs([]string{
					"0.0.0.0:8080:8080/tcp",
					"0.0.0.0:8443:8443/tcp",
				})
				mockDocker.EXPECT().ContainerCreate(gomock.Any(), &container.Config{
					Image:        subject.RepoName,
					AttachStdout: true,
					AttachStderr: true,
					ExposedPorts: exposedPorts,
					Labels:       map[string]string{"author": "pack"},
				}, &container.HostConfig{
					AutoRemove:   true,
					PortBindings: portBindings,
				}, nil, "").Return(ctr, nil)

				mockDocker.EXPECT().RunContainer(gomock.Any(), ctr.ID, gomock.Any(), gomock.Any()).Return(nil)
				mockDocker.EXPECT().ContainerRemove(gomock.Any(), ctr.ID, types.ContainerRemoveOptions{Force: true})

				err := subject.Run(ctx)
				h.AssertNil(t, err)
			})
		})
	})
}
