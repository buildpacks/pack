package build_test

import (
	"bytes"
	"context"
	"fmt"
	"math/rand"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/mount"

	dcontainer "github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"
	"github.com/heroku/color"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	"github.com/buildpacks/pack/internal/build"
	"github.com/buildpacks/pack/internal/builder"
	"github.com/buildpacks/pack/internal/container"
	h "github.com/buildpacks/pack/testhelpers"
)

// TestContainerOperations are integration tests for the container operations against a docker daemon
func TestContainerOperations(t *testing.T) {
	rand.Seed(time.Now().UTC().UnixNano())

	color.Disable(true)
	defer color.Disable(false)

	h.RequireDocker(t)

	var err error
	ctrClient, err = client.NewClientWithOpts(client.FromEnv, client.WithVersion("1.38"))
	h.AssertNil(t, err)

	spec.Run(t, "container-ops", testContainerOps, spec.Report(report.Terminal{}), spec.Sequential())
}

func testContainerOps(t *testing.T, when spec.G, it spec.S) {
	var (
		imageName       string
		outBuf, errBuf  bytes.Buffer
		isWindowsDaemon bool
	)

	it.Before(func() {
		imageName = "container-ops.test-" + h.RandString(10)
		info, err := ctrClient.Info(context.TODO())
		h.AssertNil(t, err)
		isWindowsDaemon = info.OSType == "windows"

		dockerfileContent := `FROM busybox`
		if isWindowsDaemon {
			dockerfileContent = `FROM mcr.microsoft.com/windows/nanoserver:1809`
		}

		h.CreateImage(t, ctrClient, imageName, dockerfileContent)

		h.AssertNil(t, err)
	})

	it.After(func() {
		h.DockerRmi(ctrClient, imageName)
	})

	when("#CopyDir", func() {
		it("writes contents with proper owner/permissions", func() {
			containerDir := "/some-location"
			if isWindowsDaemon {
				containerDir = `c:\some-location`
			}

			copyDirOp := build.CopyDir(filepath.Join("testdata", "fake-app"), containerDir, 123, 456, nil)
			ctx := context.Background()

			ctrCmd := []string{"ls", "-al", "/some-location"}
			if isWindowsDaemon {
				ctrCmd = []string{"cmd", "/c", `dir /q /s c:\some-location`}
			}

			ctr, err := createContainer(ctx, imageName, containerDir, ctrCmd...)
			h.AssertNil(t, err)
			defer cleanupContainer(ctx, ctr.ID)

			err = copyDirOp(ctrClient, ctx, ctr.ID, &outBuf, &errBuf)
			h.AssertNil(t, err)

			err = container.Run(ctx, ctrClient, ctr.ID, &outBuf, &errBuf)
			h.AssertNil(t, err)

			output := strings.ReplaceAll(outBuf.String(), "\r", "")

			if isWindowsDaemon {
				h.AssertContainsMatch(t, output, `
(.*)    <DIR>          ...                    .
(.*)    <DIR>          ...                    ..
(.*)                17 ...                    fake-app-file
(.*)    <SYMLINK>      ...                    fake-app-symlink \[fake-app-file\]
(.*)                 0 ...                    file-to-ignore
`)
			} else {
				perms := "-rw-r--r--"
				if runtime.GOOS == "windows" {
					perms = "-rwxrwxrwx"
				}
				h.AssertContainsMatch(t, output, fmt.Sprintf(`
%s    1 123      456 (.*) fake-app-file
%s    1 123      456 (.*) fake-app-symlink -> fake-app-file
%s    1 123      456 (.*) file-to-ignore
`, perms, "lrwxrwxrwx", perms))
			}
		})

		it("writes contents ignoring from file filter", func() {
			containerDir := "/some-location"
			if isWindowsDaemon {
				containerDir = `c:\some-location`
			}

			copyDirOp := build.CopyDir(filepath.Join("testdata", "fake-app"), containerDir, 123, 456, func(filename string) bool {
				return filepath.Base(filename) != "file-to-ignore"
			})

			ctrCmd := []string{"ls", "-al", "/some-location"}
			if isWindowsDaemon {
				ctrCmd = []string{"cmd", "/c", `dir /q /s /n c:\some-location`}
			}

			ctx := context.Background()
			ctr, err := createContainer(ctx, imageName, containerDir, ctrCmd...)
			h.AssertNil(t, err)
			defer cleanupContainer(ctx, ctr.ID)

			err = copyDirOp(ctrClient, ctx, ctr.ID, &outBuf, &errBuf)
			h.AssertNil(t, err)

			err = container.Run(ctx, ctrClient, ctr.ID, &outBuf, &errBuf)
			h.AssertNil(t, err)

			output := strings.ReplaceAll(outBuf.String(), "\r", "")

			h.AssertNotContains(t, output, "file-to-ignore")
		})

		it("writes contents from zip file", func() {
			containerDir := "/some-location"
			if isWindowsDaemon {
				containerDir = `c:\some-location`
			}

			copyDirOp := build.CopyDir(filepath.Join("testdata", "fake-app.zip"), containerDir, 123, 456, nil)

			ctrCmd := []string{"ls", "-al", "/some-location"}
			if isWindowsDaemon {
				ctrCmd = []string{"cmd", "/c", `dir /q /s /n c:\some-location`}
			}

			ctx := context.Background()
			ctr, err := createContainer(ctx, imageName, containerDir, ctrCmd...)
			h.AssertNil(t, err)
			defer cleanupContainer(ctx, ctr.ID)

			err = copyDirOp(ctrClient, ctx, ctr.ID, &outBuf, &errBuf)
			h.AssertNil(t, err)

			err = container.Run(ctx, ctrClient, ctr.ID, &outBuf, &errBuf)
			h.AssertNil(t, err)

			output := strings.ReplaceAll(outBuf.String(), "\r", "")

			if isWindowsDaemon {
				h.AssertContainsMatch(t, output, `
(.*)    <DIR>          ...                    .
(.*)    <DIR>          ...                    ..
(.*)                17 ...                    fake-app-file
`)
			} else {
				h.AssertContainsMatch(t, output, `
-rw-r--r--    1 123      456 (.*) fake-app-file
`)
			}
		})
	})

	when("#WriteStackToml", func() {
		it("writes file", func() {
			containerDir := "/some"
			containerPath := "/some/stack.toml"
			if isWindowsDaemon {
				containerDir = `c:\some`
				containerPath = `c:\some\stack.toml`
			}

			writeOp := build.WriteStackToml(containerPath, builder.StackMetadata{
				RunImage: builder.RunImageMetadata{
					Image: "image-1",
					Mirrors: []string{
						"mirror-1",
						"mirror-2",
					},
				},
			})

			ctrCmd := []string{"ls", "-al", "/some/stack.toml"}
			if isWindowsDaemon {
				ctrCmd = []string{"cmd", "/c", `dir /q /s /n c:\some\stack.toml`}
			}

			ctx := context.Background()
			ctr, err := createContainer(ctx, imageName, containerDir, ctrCmd...)
			h.AssertNil(t, err)
			defer cleanupContainer(ctx, ctr.ID)

			err = writeOp(ctrClient, ctx, ctr.ID, &outBuf, &errBuf)
			h.AssertNil(t, err)

			err = container.Run(ctx, ctrClient, ctr.ID, &outBuf, &errBuf)
			h.AssertNil(t, err)

			output := strings.ReplaceAll(outBuf.String(), "\r", "")

			if isWindowsDaemon {
				h.AssertContains(t, output, `12/31/1979  05:00 PM                69 ...                    stack.toml`)
			} else {
				h.AssertContains(t, output, `-rwxr-xr-x    1 root     root            69 Jan  1  1980 /some/stack.toml`)
			}
		})

		it("has expected contents", func() {
			containerDir := "/some"
			containerPath := "/some/stack.toml"
			if isWindowsDaemon {
				containerDir = `c:\some`
				containerPath = `c:\some\stack.toml`
			}

			writeOp := build.WriteStackToml(containerPath, builder.StackMetadata{
				RunImage: builder.RunImageMetadata{
					Image: "image-1",
					Mirrors: []string{
						"mirror-1",
						"mirror-2",
					},
				},
			})

			ctrCmd := []string{"cat", "/some/stack.toml"}
			if isWindowsDaemon {
				ctrCmd = []string{"cmd", "/c", "type", `c:\some\stack.toml`}
			}

			ctx := context.Background()
			ctr, err := createContainer(ctx, imageName, containerDir, ctrCmd...)
			h.AssertNil(t, err)
			defer cleanupContainer(ctx, ctr.ID)

			err = writeOp(ctrClient, ctx, ctr.ID, &outBuf, &errBuf)
			h.AssertNil(t, err)

			err = container.Run(ctx, ctrClient, ctr.ID, &outBuf, &errBuf)
			h.AssertNil(t, err)

			output := strings.ReplaceAll(outBuf.String(), "\r", "")
			h.AssertContains(t, output, `[run-image]
  image = "image-1"
  mirrors = ["mirror-1", "mirror-2"]
`)
		})
	})
}

func createContainer(ctx context.Context, imageName string, containerDir string, cmd ...string) (dcontainer.ContainerCreateCreatedBody, error) {
	return ctrClient.ContainerCreate(ctx,
		&dcontainer.Config{Image: imageName, Cmd: cmd},
		&dcontainer.HostConfig{
			Binds: []string{fmt.Sprintf("%s:%s", fmt.Sprintf("tests-volume-%s", h.RandString(5)), filepath.ToSlash(containerDir))},
		}, nil, "",
	)
}

func cleanupContainer(ctx context.Context, ctrID string) {
	inspect, err := ctrClient.ContainerInspect(ctx, ctrID)
	if err != nil {
		return
	}

	// remove container
	ctrClient.ContainerRemove(ctx, ctrID, types.ContainerRemoveOptions{})

	// remove volumes
	for _, m := range inspect.Mounts {
		if m.Type == mount.TypeVolume {
			ctrClient.VolumeRemove(ctx, m.Name, true)
		}
	}
}
