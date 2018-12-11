package pack_test

import (
	"bytes"
	"context"
	"crypto/md5"
	"fmt"
	"io"
	"log"
	"math/rand"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/go-connections/nat"
	"github.com/golang/mock/gomock"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	"github.com/buildpack/pack"
	"github.com/buildpack/pack/config"
	"github.com/buildpack/pack/fs"
	"github.com/buildpack/pack/mocks"
	h "github.com/buildpack/pack/testhelpers"
)

func TestRun(t *testing.T) {
	rand.Seed(time.Now().UTC().UnixNano())
	spec.Run(t, "run", testRun, spec.Parallel(), spec.Report(report.Terminal{}))
}

func testRun(t *testing.T, when spec.G, it spec.S) {
	var (
		buf            bytes.Buffer
		mockController *gomock.Controller
		mockBuild      *mocks.MockTask
		mockDocker     *mocks.MockDocker
	)

	it.Before(func() {
		mockController = gomock.NewController(t)
		mockBuild = mocks.NewMockTask(mockController)
		mockDocker = mocks.NewMockDocker(mockController)
	})

	it.After(func() {
		mockController.Finish()
	})

	when("#RunConfigFromFlags", func() {
		var (
			mockController   *gomock.Controller
			factory          *pack.BuildFactory
			mockImageFactory *mocks.MockImageFactory
		)

		it.Before(func() {
			mockController = gomock.NewController(t)
			mockImageFactory = mocks.NewMockImageFactory(mockController)
			factory = &pack.BuildFactory{
				Cli:    mockDocker,
				Stdout: &buf,
				Stderr: &buf,
				Log:    log.New(&buf, "", log.LstdFlags|log.Lshortfile),
				FS:     &fs.FS{},
				ImageFactory: mockImageFactory,
				Config: &config.Config{
					Stacks: []config.Stack{
						{
							ID:        "some.stack.id",
							RunImages: []string{"some/run", "registry.com/some/run"},
						},
					},
				},
			}

		})

		it.After(func() {
			mockController.Finish()
		})

		it("creates a RunConfig derived from a BuildConfig", func() {
			mockBuilderImage := mocks.NewMockImage(mockController)
			mockBuilderImage.EXPECT().Label("io.buildpacks.stack.id").Return("some.stack.id", nil)
			mockImageFactory.EXPECT().NewLocal("some/builder", true).Return(mockBuilderImage, nil)

			mockRunImage := mocks.NewMockImage(mockController)
			mockRunImage.EXPECT().Label("io.buildpacks.stack.id").Return("some.stack.id", nil)
			mockImageFactory.EXPECT().NewLocal("some/run", true).Return(mockRunImage, nil)

			run, err := factory.RunConfigFromFlags(&pack.RunFlags{
				BuildFlags: pack.BuildFlags{
					AppDir:   "acceptance/testdata/node_app",
					Builder:  "some/builder",
					RunImage: "some/run",
				},
				Port: "1370",
			})
			h.AssertNil(t, err)

			absAppDir, _ := filepath.Abs("acceptance/testdata/node_app")
			absAppDirMd5 := fmt.Sprintf("pack.local/run/%x", md5.Sum([]byte(absAppDir)))
			h.AssertEq(t, run.RepoName, absAppDirMd5)
			h.AssertEq(t, run.Port, "1370")

			build, ok := run.Build.(*pack.BuildConfig)
			h.AssertEq(t, ok, true)
			for _, field := range []string{
				"RepoName",
				"Cli",
				"Stdout",
				"Stderr",
				"Log",
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
			subject    *pack.RunConfig
			ctr        container.ContainerCreateCreatedBody
			stopCh     chan struct{}
			makeStopCh func() <-chan struct{}
		)

		it.Before(func() {
			stopCh = make(chan struct{}, 1)
			makeStopCh = func() <-chan struct{} {
				return stopCh
			}

			subject = &pack.RunConfig{
				Build:    mockBuild,
				RepoName: "pack.local/run/346ffb210a2c6d138c8d058d6d4025a0",
				Port:     "1370",
				Cli:      mockDocker,
				Log:      log.New(&buf, "", log.LstdFlags|log.Lshortfile),
				Stdout:   &buf,
				Stderr:   &buf,
			}
			ctr = container.ContainerCreateCreatedBody{
				ID: "29aef5a011dd",
			}
		})

		it("builds an image and runs it", func() {
			mockBuild.EXPECT().Run().Return(nil)

			exposedPorts, portBindings, _ := nat.ParsePortSpecs([]string{"127.0.0.1:1370:1370/tcp"})
			mockDocker.EXPECT().ContainerCreate(gomock.Any(), &container.Config{
				Image:        subject.RepoName,
				AttachStdout: true,
				AttachStderr: true,
				ExposedPorts: exposedPorts,
			}, &container.HostConfig{
				AutoRemove:   true,
				PortBindings: portBindings,
			}, nil, "").Return(ctr, nil)

			mockDocker.EXPECT().ContainerRemove(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).Times(0)
			mockDocker.EXPECT().RunContainer(gomock.Any(), ctr.ID, subject.Stdout, subject.Stderr).Return(nil)

			err := subject.Run(makeStopCh)
			h.AssertNil(t, err)

			h.AssertContains(t, buf.String(), "Starting container listening at http://localhost:1370/")
		})

		when("the build fails", func() {
			it("exits without running", func() {
				expected := fmt.Errorf("build error")
				mockBuild.EXPECT().Run().Return(expected)

				mockDocker.EXPECT().ContainerCreate(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Times(0)
				mockDocker.EXPECT().ContainerRemove(gomock.Any(), gomock.Any(), gomock.Any()).Times(0)
				mockDocker.EXPECT().RunContainer(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Times(0)

				err := subject.Run(makeStopCh)
				h.AssertSameInstance(t, err, expected)
			})
		})

		when("the process is terminated", func() {
			it("stops the running container and cleans up", func() {
				syncCh := make(chan struct{})

				mockBuild.EXPECT().Run().Return(nil)
				mockDocker.EXPECT().ContainerCreate(gomock.Any(), gomock.Any(), gomock.Any(), nil, "").Return(ctr, nil)

				mockDocker.EXPECT().ContainerRemove(gomock.Any(), ctr.ID, types.ContainerRemoveOptions{Force: true}).DoAndReturn(func(ctx context.Context, containerID string, options types.ContainerRemoveOptions) error {
					syncCh <- struct{}{}
					return nil
				})
				mockDocker.EXPECT().RunContainer(gomock.Any(), ctr.ID, subject.Stdout, subject.Stderr).DoAndReturn(func(ctx context.Context, id string, stdout io.Writer, stderr io.Writer) error {
					stopCh <- struct{}{}
					// wait for ContainerRemove to be called
					<-syncCh
					return nil
				})

				err := subject.Run(makeStopCh)
				h.AssertNil(t, err)
			})
		})

		when("the port is not specified", func() {
			it.Before(func() {
				subject.Port = ""
			})

			it("gets exposed ports from the built image", func() {
				mockBuild.EXPECT().Run().Return(nil)

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
				}, &container.HostConfig{
					AutoRemove:   true,
					PortBindings: portBindings,
				}, nil, "").Return(ctr, nil)

				mockDocker.EXPECT().RunContainer(gomock.Any(), ctr.ID, subject.Stdout, subject.Stderr).Return(nil)

				err := subject.Run(makeStopCh)
				h.AssertNil(t, err)
			})
		})
		when("custom ports bindings are defined", func() {
			it("binds simple ports from localhost to the container on the same port", func() {
				mockBuild.EXPECT().Run().Return(nil)

				subject.Port = "1370"
				exposedPorts, portBindings, _ := nat.ParsePortSpecs([]string{
					"127.0.0.1:1370:1370/tcp",
				})
				mockDocker.EXPECT().ContainerCreate(gomock.Any(), &container.Config{
					Image:        subject.RepoName,
					AttachStdout: true,
					AttachStderr: true,
					ExposedPorts: exposedPorts,
				}, &container.HostConfig{
					AutoRemove:   true,
					PortBindings: portBindings,
				}, nil, "").Return(ctr, nil)

				mockDocker.EXPECT().RunContainer(gomock.Any(), ctr.ID, subject.Stdout, subject.Stderr).Return(nil)

				err := subject.Run(makeStopCh)
				h.AssertNil(t, err)
			})
			it("binds each port to the container", func() {
				mockBuild.EXPECT().Run().Return(nil)

				subject.Port = "0.0.0.0:8080:8080/tcp, 0.0.0.0:8443:8443/tcp"
				exposedPorts, portBindings, _ := nat.ParsePortSpecs([]string{
					"0.0.0.0:8080:8080/tcp",
					"0.0.0.0:8443:8443/tcp",
				})
				mockDocker.EXPECT().ContainerCreate(gomock.Any(), &container.Config{
					Image:        subject.RepoName,
					AttachStdout: true,
					AttachStderr: true,
					ExposedPorts: exposedPorts,
				}, &container.HostConfig{
					AutoRemove:   true,
					PortBindings: portBindings,
				}, nil, "").Return(ctr, nil)

				mockDocker.EXPECT().RunContainer(gomock.Any(), ctr.ID, subject.Stdout, subject.Stderr).Return(nil)

				err := subject.Run(makeStopCh)
				h.AssertNil(t, err)
			})
		})
	})
}
