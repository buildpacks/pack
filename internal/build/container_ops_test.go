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
		imageName string
		osType    string
	)

	it.Before(func() {
		imageName = "container-ops.test-" + h.RandString(10)

		info, err := ctrClient.Info(context.TODO())
		h.AssertNil(t, err)
		osType = info.OSType

		dockerfileContent := `FROM busybox`
		if osType == "windows" {
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
			containerDir := "/some-vol"
			if osType == "windows" {
				containerDir = `c:\some-vol`
			}

			ctrCmd := []string{"ls", "-al", "/some-vol"}
			if osType == "windows" {
				ctrCmd = []string{"cmd", "/c", `dir /q /s c:\some-vol`}
			}

			ctx := context.Background()
			ctr, err := createContainer(ctx, imageName, containerDir, osType, ctrCmd...)
			h.AssertNil(t, err)
			defer cleanupContainer(ctx, ctr.ID)

			copyDirOp := build.CopyDir(filepath.Join("testdata", "fake-app"), containerDir, 123, 456, osType, nil)

			var outBuf, errBuf bytes.Buffer
			err = copyDirOp(ctrClient, ctx, ctr.ID, &outBuf, &errBuf)
			h.AssertNil(t, err)

			err = container.Run(ctx, ctrClient, ctr.ID, &outBuf, &errBuf)
			h.AssertNil(t, err)

			h.AssertEq(t, errBuf.String(), "")
			if osType == "windows" {
				// Expected WCOW results
				h.AssertContainsMatch(t, strings.ReplaceAll(outBuf.String(), "\r", ""), `
(.*)    <DIR>          ...                    .
(.*)    <DIR>          ...                    ..
(.*)                17 ...                    fake-app-file
(.*)    <SYMLINK>      ...                    fake-app-symlink \[fake-app-file\]
(.*)                 0 ...                    file-to-ignore
`)
			} else {
				if runtime.GOOS == "windows" {
					// Expected LCOW results
					h.AssertContainsMatch(t, outBuf.String(), `
-rwxrwxrwx    1 123      456 (.*) fake-app-file
lrwxrwxrwx    1 123      456 (.*) fake-app-symlink -> fake-app-file
-rwxrwxrwx    1 123      456 (.*) file-to-ignore
`)
				} else {
					// Expected results
					h.AssertContainsMatch(t, outBuf.String(), `
-rw-r--r--    1 123      456 (.*) fake-app-file
lrwxrwxrwx    1 123      456 (.*) fake-app-symlink -> fake-app-file
-rw-r--r--    1 123      456 (.*) file-to-ignore
`)
				}
			}
		})

		it("writes contents ignoring from file filter", func() {
			containerDir := "/some-vol"
			if osType == "windows" {
				containerDir = `c:\some-vol`
			}

			ctrCmd := []string{"ls", "-al", "/some-vol"}
			if osType == "windows" {
				ctrCmd = []string{"cmd", "/c", `dir /q /s /n c:\some-vol`}
			}

			ctx := context.Background()
			ctr, err := createContainer(ctx, imageName, containerDir, osType, ctrCmd...)
			h.AssertNil(t, err)
			defer cleanupContainer(ctx, ctr.ID)

			copyDirOp := build.CopyDir(filepath.Join("testdata", "fake-app"), containerDir, 123, 456, osType, func(filename string) bool {
				return filepath.Base(filename) != "file-to-ignore"
			})

			var outBuf, errBuf bytes.Buffer
			err = copyDirOp(ctrClient, ctx, ctr.ID, &outBuf, &errBuf)
			h.AssertNil(t, err)

			err = container.Run(ctx, ctrClient, ctr.ID, &outBuf, &errBuf)
			h.AssertNil(t, err)

			h.AssertEq(t, errBuf.String(), "")
			h.AssertContains(t, outBuf.String(), "fake-app-file")
			h.AssertNotContains(t, outBuf.String(), "file-to-ignore")
		})

		it("writes contents from zip file", func() {
			containerDir := "/some-vol"
			if osType == "windows" {
				containerDir = `c:\some-vol`
			}

			ctrCmd := []string{"ls", "-al", "/some-vol"}
			if osType == "windows" {
				ctrCmd = []string{"cmd", "/c", `dir /q /s /n c:\some-vol`}
			}

			ctx := context.Background()
			ctr, err := createContainer(ctx, imageName, containerDir, osType, ctrCmd...)
			h.AssertNil(t, err)
			defer cleanupContainer(ctx, ctr.ID)

			copyDirOp := build.CopyDir(filepath.Join("testdata", "fake-app.zip"), containerDir, 123, 456, osType, nil)

			var outBuf, errBuf bytes.Buffer
			err = copyDirOp(ctrClient, ctx, ctr.ID, &outBuf, &errBuf)
			h.AssertNil(t, err)

			err = container.Run(ctx, ctrClient, ctr.ID, &outBuf, &errBuf)
			h.AssertNil(t, err)

			h.AssertEq(t, errBuf.String(), "")
			if osType == "windows" {
				h.AssertContainsMatch(t, strings.ReplaceAll(outBuf.String(), "\r", ""), `
(.*)    <DIR>          ...                    .
(.*)    <DIR>          ...                    ..
(.*)                17 ...                    fake-app-file
`)
			} else {
				h.AssertContainsMatch(t, outBuf.String(), `
-rw-r--r--    1 123      456 (.*) fake-app-file
`)
			}
		})
	})

	when("#WriteStackToml", func() {
		it("writes file", func() {
			containerDir := "/layers-vol"
			containerPath := "/layers-vol/stack.toml"
			if osType == "windows" {
				containerDir = `c:\layers-vol`
				containerPath = `c:\layers-vol\stack.toml`
			}

			ctrCmd := []string{"ls", "-al", "/layers-vol/stack.toml"}
			if osType == "windows" {
				ctrCmd = []string{"cmd", "/c", `dir /q /n c:\layers-vol\stack.toml`}
			}
			ctx := context.Background()
			ctr, err := createContainer(ctx, imageName, containerDir, osType, ctrCmd...)
			h.AssertNil(t, err)
			defer cleanupContainer(ctx, ctr.ID)

			writeOp := build.WriteStackToml(containerPath, builder.StackMetadata{
				RunImage: builder.RunImageMetadata{
					Image: "image-1",
					Mirrors: []string{
						"mirror-1",
						"mirror-2",
					},
				},
			}, osType)

			var outBuf, errBuf bytes.Buffer
			err = writeOp(ctrClient, ctx, ctr.ID, &outBuf, &errBuf)
			h.AssertNil(t, err)

			err = container.Run(ctx, ctrClient, ctr.ID, &outBuf, &errBuf)
			h.AssertNil(t, err)

			h.AssertEq(t, errBuf.String(), "")
			if osType == "windows" {
				h.AssertContains(t, outBuf.String(), `01/01/1980  12:00 AM                69 ...                    stack.toml`)
			} else {
				h.AssertContains(t, outBuf.String(), `-rwxr-xr-x    1 root     root            69 Jan  1  1980 /layers-vol/stack.toml`)
			}
		})

		it("has expected contents", func() {
			containerDir := "/layers-vol"
			containerPath := "/layers-vol/stack.toml"
			if osType == "windows" {
				containerDir = `c:\layers-vol`
				containerPath = `c:\layers-vol\stack.toml`
			}

			ctrCmd := []string{"cat", "/layers-vol/stack.toml"}
			if osType == "windows" {
				ctrCmd = []string{"cmd", "/c", `type c:\layers-vol\stack.toml`}
			}

			ctx := context.Background()
			ctr, err := createContainer(ctx, imageName, containerDir, osType, ctrCmd...)
			h.AssertNil(t, err)
			defer cleanupContainer(ctx, ctr.ID)

			writeOp := build.WriteStackToml(containerPath, builder.StackMetadata{
				RunImage: builder.RunImageMetadata{
					Image: "image-1",
					Mirrors: []string{
						"mirror-1",
						"mirror-2",
					},
				},
			}, osType)

			var outBuf, errBuf bytes.Buffer
			err = writeOp(ctrClient, ctx, ctr.ID, &outBuf, &errBuf)
			h.AssertNil(t, err)

			err = container.Run(ctx, ctrClient, ctr.ID, &outBuf, &errBuf)
			h.AssertNil(t, err)

			h.AssertEq(t, errBuf.String(), "")
			h.AssertContains(t, outBuf.String(), `[run-image]
  image = "image-1"
  mirrors = ["mirror-1", "mirror-2"]
`)
		})
	})

	when("#EnsureVolumeAccess", func() {
		it("changes owner of volume", func() {
			h.SkipIf(t, osType != "windows", "no-op for linux")

			ctx := context.Background()

			ctrCmd := []string{"ls", "-al", "/my-volume"}
			containerDir := "/my-volume"
			if osType == "windows" {
				ctrCmd = []string{"cmd", "/c", `icacls c:\my-volume`}
				containerDir = `c:\my-volume`
			}

			ctr, err := createContainer(ctx, imageName, containerDir, osType, ctrCmd...)
			h.AssertNil(t, err)
			defer cleanupContainer(ctx, ctr.ID)

			inspect, err := ctrClient.ContainerInspect(ctx, ctr.ID)
			if err != nil {
				return
			}

			// use container's current volumes
			var ctrVolumes []string
			for _, m := range inspect.Mounts {
				if m.Type == mount.TypeVolume {
					ctrVolumes = append(ctrVolumes, m.Name)
				}
			}

			var outBuf, errBuf bytes.Buffer

			// reuse same volume twice to demonstrate multiple ops
			initVolumeOp := build.EnsureVolumeAccess(123, 456, osType, ctrVolumes[0], ctrVolumes[0])
			err = initVolumeOp(ctrClient, ctx, ctr.ID, &outBuf, &errBuf)
			h.AssertNil(t, err)
			err = container.Run(ctx, ctrClient, ctr.ID, &outBuf, &errBuf)
			h.AssertNil(t, err)

			h.AssertEq(t, errBuf.String(), "")
			h.AssertContains(t, outBuf.String(), `BUILTIN\Users:(OI)(CI)(F)`)
		})
	})
}

func createContainer(ctx context.Context, imageName, containerDir, osType string, cmd ...string) (dcontainer.ContainerCreateCreatedBody, error) {
	isolationType := dcontainer.IsolationDefault
	if osType == "windows" {
		isolationType = dcontainer.IsolationProcess
	}

	return ctrClient.ContainerCreate(ctx,
		&dcontainer.Config{
			Image: imageName,
			Cmd:   cmd,
		},
		&dcontainer.HostConfig{
			Binds:     []string{fmt.Sprintf("%s:%s", fmt.Sprintf("tests-volume-%s", h.RandString(5)), filepath.ToSlash(containerDir))},
			Isolation: isolationType,
		}, nil, nil, "",
	)
}

func cleanupContainer(ctx context.Context, ctrID string) {
	inspect, err := ctrClient.ContainerInspect(ctx, ctrID)
	if err != nil {
		return
	}

	// remove container
	err = ctrClient.ContainerRemove(ctx, ctrID, types.ContainerRemoveOptions{})
	if err != nil {
		return
	}

	// remove volumes
	for _, m := range inspect.Mounts {
		if m.Type == mount.TypeVolume {
			err = ctrClient.VolumeRemove(ctx, m.Name, true)
			if err != nil {
				return
			}
		}
	}
}
