package build_test

import (
	"bytes"
	"context"
	"math/rand"
	"path"
	"path/filepath"
	"testing"
	"time"

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

	info, err := ctrClient.Info(context.TODO())
	h.AssertNil(t, err)
	h.SkipIf(t, info.OSType == "windows", "These tests are not yet compatible with Windows-based containers")

	spec.Run(t, "container-ops", testContainerOps, spec.Report(report.Terminal{}), spec.Sequential())
}

func testContainerOps(t *testing.T, when spec.G, it spec.S) {
	var (
		imageName      string
		outBuf, errBuf bytes.Buffer
	)

	it.Before(func() {
		imageName = "container-ops.test-" + h.RandString(10)
		h.CreateImage(t, ctrClient, imageName, `FROM busybox`)
	})

	it.After(func() {
		h.DockerRmi(ctrClient, imageName)
	})

	when("#CopyDir", func() {
		it("writes contents with proper permissions", func() {
			copyDirOp := build.CopyDir(filepath.Join("testdata", "fake-app"), "/some-location", 123, 456, nil)
			ctx := context.Background()

			ctr, err := createContainer(ctx, imageName, "ls", "-al", "/some-location")
			h.AssertNil(t, err)

			err = copyDirOp(ctrClient, ctx, ctr.ID)
			h.AssertNil(t, err)

			err = container.Run(ctx, ctrClient, ctr.ID, &outBuf, &errBuf)
			h.AssertNil(t, err)

			output := outBuf.String()
			h.AssertContainsMatch(t, output, `
-rw-r--r--    1 123      456 (.*) fake-app-file
-rw-r--r--    1 123      456 (.*) file-to-ignore
`)
		})

		it("writes contents ignoring from file filter", func() {
			copyDirOp := build.CopyDir(filepath.Join("testdata", "fake-app"), "/some-location", 123, 456, func(filename string) bool {
				return path.Base(filename) != "file-to-ignore"
			})
			ctx := context.Background()

			ctr, err := createContainer(ctx, imageName, "ls", "-al", "/some-location")
			h.AssertNil(t, err)

			err = copyDirOp(ctrClient, ctx, ctr.ID)
			h.AssertNil(t, err)

			err = container.Run(ctx, ctrClient, ctr.ID, &outBuf, &errBuf)
			h.AssertNil(t, err)

			output := outBuf.String()
			h.AssertNotContains(t, output, "file-to-ignore")
		})

		it("writes contents from zip file", func() {
			copyDirOp := build.CopyDir(filepath.Join("testdata", "fake-app.zip"), "/some-location", 123, 456, nil)
			ctx := context.Background()

			ctr, err := createContainer(ctx, imageName, "ls", "-al", "/some-location")
			h.AssertNil(t, err)

			err = copyDirOp(ctrClient, ctx, ctr.ID)
			h.AssertNil(t, err)

			err = container.Run(ctx, ctrClient, ctr.ID, &outBuf, &errBuf)
			h.AssertNil(t, err)

			output := outBuf.String()
			h.AssertContainsMatch(t, output, `
-rw-r--r--    1 123      456 (.*) fake-app-file
`)
		})
	})

	when("#WriteStackToml", func() {
		it("writes file", func() {
			writeOp := build.WriteStackToml("/some/stack.toml", builder.StackMetadata{
				RunImage: builder.RunImageMetadata{
					Image: "image-1",
					Mirrors: []string{
						"mirror-1",
						"mirror-2",
					},
				},
			})
			ctx := context.Background()
			ctr, err := createContainer(ctx, imageName, "ls", "-al", "/some/stack.toml")
			h.AssertNil(t, err)

			err = writeOp(ctrClient, ctx, ctr.ID)
			h.AssertNil(t, err)

			err = container.Run(ctx, ctrClient, ctr.ID, &outBuf, &errBuf)
			h.AssertNil(t, err)

			output := outBuf.String()
			h.AssertContains(t, output, `-rwxr-xr-x    1 root     root            69 Jan  1  1980 /some/stack.toml`)
		})

		it("has expected contents", func() {
			writeOp := build.WriteStackToml("/some/stack.toml", builder.StackMetadata{
				RunImage: builder.RunImageMetadata{
					Image: "image-1",
					Mirrors: []string{
						"mirror-1",
						"mirror-2",
					},
				},
			})
			ctx := context.Background()
			ctr, err := createContainer(ctx, imageName, "cat", "/some/stack.toml")
			h.AssertNil(t, err)

			err = writeOp(ctrClient, ctx, ctr.ID)
			h.AssertNil(t, err)

			err = container.Run(ctx, ctrClient, ctr.ID, &outBuf, &errBuf)
			h.AssertNil(t, err)

			output := outBuf.String()
			h.AssertContains(t, output, `[run-image]
  image = "image-1"
  mirrors = ["mirror-1", "mirror-2"]
`)
		})
	})
}

func createContainer(ctx context.Context, imageName string, cmd ...string) (dcontainer.ContainerCreateCreatedBody, error) {
	return ctrClient.ContainerCreate(ctx,
		&dcontainer.Config{Image: imageName, Cmd: cmd},
		&dcontainer.HostConfig{}, nil, "",
	)
}
