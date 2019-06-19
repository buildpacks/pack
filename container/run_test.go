package container_test

import (
	"bytes"
	"context"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	dockertypes "github.com/docker/docker/api/types"
	dcontainer "github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	"github.com/buildpack/imgutil"
	"github.com/buildpack/pack/container"
	"github.com/buildpack/pack/internal/archive"
	h "github.com/buildpack/pack/testhelpers"
)

var (
	repoName string
	docker   *client.Client
)

func TestContainerRun(t *testing.T) {
	h.RequireDocker(t)

	var err error
	docker, err = client.NewClientWithOpts(client.FromEnv, client.WithVersion("1.38"))
	h.AssertNil(t, err)

	repoName = "lifecycle.test." + h.RandString(10)
	CreateFakeLifecycleImage(t, docker, repoName)
	defer h.DockerRmi(docker, repoName)

	spec.Run(t, "ContainerRun", testContainerRun, spec.Parallel(), spec.Report(report.Terminal{}))
}

func testContainerRun(t *testing.T, when spec.G, it spec.S) {
	when("#Run", func() {
		var (
			outBuf, errBuf bytes.Buffer
			ctr            dcontainer.ContainerCreateCreatedBody
		)

		it.Before(func() {
			var err error

			image, err := imgutil.NewLocalImage(repoName, docker)
			h.AssertNil(t, err)

			ctrConf := &dcontainer.Config{
				Image:  image.Name(),
				Labels: map[string]string{"author": "pack"},
			}

			hostConf := &dcontainer.HostConfig{}

			ctr, err = docker.ContainerCreate(context.Background(), ctrConf, hostConf, nil, "")
			h.AssertNil(t, err)
		})

		it("runs a container", func() {
			err := container.Run(context.Background(), docker, ctr.ID, &outBuf, &errBuf)
			h.AssertNil(t, err)
			h.AssertContains(t, outBuf.String(), "Hello From Docker")
		})
	})
}

func CreateFakeLifecycleImage(t *testing.T, docker *client.Client, repoName string) {
	ctx := context.Background()

	wd, err := os.Getwd()
	h.AssertNil(t, err)
	buildContext, _ := archive.CreateTarReader(filepath.Join(wd, "testdata", "fake-run"), "/", 0, 0, -1)

	res, err := docker.ImageBuild(ctx, buildContext, dockertypes.ImageBuildOptions{
		Tags:        []string{repoName},
		Remove:      true,
		ForceRemove: true,
	})
	h.AssertNil(t, err)

	io.Copy(ioutil.Discard, res.Body)
	res.Body.Close()
}
