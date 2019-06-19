package container_test

import (
	"bytes"
	"context"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"
	"time"

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

var repos = map[string]struct {
	repoName   string
	testFolder string
	ctrID      string
}{
	"default": {
		repoName:   "lifecycle.test." + h.RandString(10),
		testFolder: "fake-run",
	},
	"slow": {
		repoName:   "lifecycle.test." + h.RandString(10),
		testFolder: "slow-run",
	},
}

var (
	repoName     string
	slowRepoName string
	docker       *client.Client
)

func TestContainerRun(t *testing.T) {
	h.RequireDocker(t)

	var err error
	docker, err = client.NewClientWithOpts(client.FromEnv, client.WithVersion("1.38"))
	h.AssertNil(t, err)

	for _, r := range repos {
		CreateFakeImage(t, docker, r.testFolder, r.repoName)
		defer h.DockerRmi(docker, r.repoName)
	}

	spec.Run(t, "ContainerRun", testContainerRun, spec.Parallel(), spec.Report(report.Terminal{}))
}

func testContainerRun(t *testing.T, when spec.G, it spec.S) {
	when("#Run", func() {
		var (
			outBuf, errBuf bytes.Buffer
		)

		it.Before(func() {
			for k, r := range repos {
				image, err := imgutil.NewLocalImage(r.repoName, docker)
				h.AssertNil(t, err)

				hostConf := &dcontainer.HostConfig{}
				ctrConf := &dcontainer.Config{
					Image:  image.Name(),
					Labels: map[string]string{"author": "pack"},
				}

				ctr, err := docker.ContainerCreate(context.Background(), ctrConf, hostConf, nil, "")
				h.AssertNil(t, err)
				r.ctrID = ctr.ID
				repos[k] = r
			}
		})

		it("runs a container", func() {
			err := container.Run(context.Background(), docker, repos["default"].ctrID, &outBuf, &errBuf)
			h.AssertNil(t, err)
			h.AssertContains(t, outBuf.String(), "Hello From Docker")
		})

		it("can cancel a running container", func() {
			ctx, cancel := context.WithCancel(context.Background())
			go func() {
				time.Sleep(2 * time.Second)
				cancel()
			}()

			err := container.Run(ctx, docker, repos["slow"].ctrID, &outBuf, &errBuf)
			h.AssertNil(t, err)
			h.AssertContains(t, outBuf.String(), "Hello From Docker")
			h.AssertNotContains(t, outBuf.String(), "Hello after sleeping")
		})
	})
}

func CreateFakeImage(t *testing.T, docker *client.Client, testRun, repoName string) {
	ctx := context.Background()

	wd, err := os.Getwd()
	h.AssertNil(t, err)
	buildContext, _ := archive.CreateTarReader(filepath.Join(wd, "testdata", testRun), "/", 0, 0, -1)

	res, err := docker.ImageBuild(ctx, buildContext, dockertypes.ImageBuildOptions{
		Tags:        []string{repoName},
		Remove:      true,
		ForceRemove: true,
	})
	h.AssertNil(t, err)

	io.Copy(ioutil.Discard, res.Body)
	res.Body.Close()
}
