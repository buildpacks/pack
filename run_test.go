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

	"github.com/buildpack/pack"
	"github.com/buildpack/pack/config"
	"github.com/buildpack/pack/fs"
	"github.com/buildpack/pack/mocks"
	"github.com/docker/docker/api/types"
	dockertypes "github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	dockercontainer "github.com/docker/docker/api/types/container"
	"github.com/docker/go-connections/nat"
	"github.com/golang/mock/gomock"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"
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
		mockImages     *mocks.MockImages
	)

	it.Before(func() {
		mockController = gomock.NewController(t)
		mockBuild = mocks.NewMockTask(mockController)
		mockDocker = mocks.NewMockDocker(mockController)
		mockImages = mocks.NewMockImages(mockController)
	})

	it.After(func() {
		mockController.Finish()
	})

	when("#RunConfigFromFlags", func() {
		var factory *pack.BuildFactory

		it.Before(func() {
			factory = &pack.BuildFactory{
				Cli:    mockDocker,
				Stdout: &buf,
				Stderr: &buf,
				Log:    log.New(&buf, "", log.LstdFlags|log.Lshortfile),
				FS:     &fs.FS{},
				Images: mockImages,
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

		it("creates a RunConfig derived from a BuildConfig", func() {
			mockDocker.EXPECT().PullImage("some/builder")
			mockDocker.EXPECT().ImageInspectWithRaw(gomock.Any(), "some/builder").Return(dockertypes.ImageInspect{
				Config: &dockercontainer.Config{
					Labels: map[string]string{"io.buildpacks.stack.id": "some.stack.id"},
				},
			}, nil, nil)
			mockDocker.EXPECT().PullImage("some/run")
			mockDocker.EXPECT().ImageInspectWithRaw(gomock.Any(), "some/run").Return(dockertypes.ImageInspect{
				Config: &dockercontainer.Config{
					Labels: map[string]string{"io.buildpacks.stack.id": "some.stack.id"},
				},
			}, nil, nil)

			run, err := factory.RunConfigFromFlags(&pack.RunFlags{
				AppDir:   "acceptance/testdata/node_app",
				Builder:  "some/builder",
				RunImage: "some/run",
				Port:     "1370",
			})
			assertNil(t, err)

			absAppDir, _ := filepath.Abs("acceptance/testdata/node_app")
			h := md5.New()
			io.WriteString(h, absAppDir)
			absAppDirMd5 := fmt.Sprintf("%x", h.Sum(nil))
			assertEq(t, run.AppDir, absAppDir)
			assertEq(t, run.RepoName, absAppDirMd5)
			assertEq(t, run.Builder, "some/builder")
			assertEq(t, run.RunImage, "some/run")
			assertEq(t, run.Port, "1370")

			build, ok := run.Build.(*pack.BuildConfig)
			assertEq(t, ok, true)
			for _, field := range []string{
				"AppDir",
				"Builder",
				"RunImage",
				"RepoName",
				"Cli",
				"Stdout",
				"Stderr",
				"Log",
				"FS",
				"Config",
				"Images",
				"WorkspaceVolume",
				"CacheVolume",
			} {
				assertSameInstance(
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
				RepoName: "346ffb210a2c6d138c8d058d6d4025a0",
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

			exposedPorts, portBindings, _ := nat.ParsePortSpecs([]string{"127.0.0.1:1370:8080/tcp"})
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
			assertNil(t, err)

			assertContains(t, buf.String(), "Starting container listening at http://localhost:1370/")
		})

		when("the build fails", func() {
			it("exits without running", func() {
				expected := fmt.Errorf("build error")
				mockBuild.EXPECT().Run().Return(expected)

				mockDocker.EXPECT().ContainerCreate(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Times(0)
				mockDocker.EXPECT().ContainerRemove(gomock.Any(), gomock.Any(), gomock.Any()).Times(0)
				mockDocker.EXPECT().RunContainer(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Times(0)

				err := subject.Run(makeStopCh)
				assertSameInstance(t, err, expected)
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
				assertNil(t, err)
			})
		})

		when("custom ports bindings are defined", func() {
			it("binds simple ports from localhost to the container on 8080", func() {
				mockBuild.EXPECT().Run().Return(nil)

				subject.Port = "1370"
				exposedPorts, portBindings, _ := nat.ParsePortSpecs([]string{
					"127.0.0.1:1370:8080/tcp",
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
				assertNil(t, err)
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
				assertNil(t, err)
			})
		})
	})

}
