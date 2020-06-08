// +build acceptance

package managers

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	dockertypes "github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"
	"github.com/docker/docker/pkg/stdcopy"
	"github.com/docker/go-connections/nat"

	"github.com/buildpacks/pack/acceptance/assertions"
	h "github.com/buildpacks/pack/testhelpers"
)

type TestContainerManager struct {
	testObject *testing.T
	assert     assertions.AssertionManager
	dockerCli  *client.Client
}

//nolint:whitespace // A leading line of whitespace is left after a method declaration with multi-line arguments
func NewTestContainerManager(
	t *testing.T,
	assert assertions.AssertionManager,
	dockerCli *client.Client,
) TestContainerManager {

	return TestContainerManager{
		testObject: t,
		assert:     assert,
		dockerCli:  dockerCli,
	}
}

func (c TestContainerManager) HostOS() string {
	c.testObject.Helper()

	daemonInfo, err := c.dockerCli.Info(context.Background())
	c.assert.Nil(err)

	return daemonInfo.OSType
}

func (c TestContainerManager) CleanupTaskForImageByName(imageName string) func() {
	c.testObject.Helper()

	imageID := c.ImageID(imageName)

	return func() {
		c.RemoveImages(imageID)
	}
}

func (c TestContainerManager) RemoveImages(repoNames ...string) {
	// TODO: Error ignored - nested packages, for example, don't exist - is this more of a `force`?
	h.DockerRmi(c.dockerCli, repoNames...)
}

func (c TestContainerManager) RemoveImagesSucceeds(repoNames ...string) {
	err := h.DockerRmi(c.dockerCli, repoNames...)

	c.assert.Nil(err)
}

func (c TestContainerManager) ImageID(imageName string) string {
	c.testObject.Helper()

	inspect, _, err := c.dockerCli.ImageInspectWithRaw(context.Background(), imageName)
	c.assert.Nil(err)

	return inspect.ID
}

func (c TestContainerManager) ImageIDForReference(imageName string) string {
	c.testObject.Helper()

	return strings.TrimPrefix(c.ImageID(imageName), "sha256:")
}

func (c TestContainerManager) TopLayerDiffID(imageName string) string {
	c.testObject.Helper()

	layers := c.RootFSLayers(imageName)
	layerCount := len(layers)

	if layerCount < 1 {
		c.testObject.Fatalf("image '%s' has no layers", imageName)
	}

	return layers[layerCount-1]
}

func (c TestContainerManager) RootFSLayers(imageName string) []string {
	inspect, _, err := c.dockerCli.ImageInspectWithRaw(context.Background(), imageName)
	c.assert.Nil(err)

	return inspect.RootFS.Layers
}

func (c TestContainerManager) ImageLabel(imageName, labelName string) string {
	c.testObject.Helper()

	inspect, _, err := c.dockerCli.ImageInspectWithRaw(context.Background(), imageName)
	c.assert.Nil(err)

	label, ok := inspect.Config.Labels[labelName]
	if !ok {
		c.testObject.Fatalf("expected label %s to exist", labelName)
	}

	return label
}

const (
	customRunImageFormat = `
FROM %s
USER root
RUN echo "custom-run" > /custom-run.txt
USER pack
`
	differentStackRunImageFormat = `
FROM %s
LABEL io.buildpacks.stack.id=other.stack.id
USER pack
`
)

func (c TestContainerManager) CreateCustomRunImageOnRemote(registry *h.TestRegistryConfig) string {
	c.testObject.Helper()

	return c.createImageOnRemote(registry, randomRunImageName(), customRunImageFormat)
}

func (c TestContainerManager) CreateDifferentStackRunImageOnRemote(registry *h.TestRegistryConfig) string {
	c.testObject.Helper()

	return c.createImageOnRemote(registry, randomRunImageName(), differentStackRunImageFormat)
}

func randomRunImageName() string {
	return "custom-run-image" + h.RandString(10)
}

func (c TestContainerManager) createImageOnRemote(registry *h.TestRegistryConfig, name, fileFormat string) string {
	c.testObject.Helper()

	return h.CreateImageOnRemote(c.testObject, c.dockerCli, registry, name, fmt.Sprintf(fileFormat, RunImage))
}

type TestContainer struct {
	testObject *testing.T
	dockerCli  *client.Client
	assert     assertions.AssertionManager
	name       string
}

func (c TestContainerManager) RunDockerImageExposePort(containerName, repoName string) assertions.Container {
	c.testObject.Helper()
	ctx := context.Background()

	ctr, err := c.dockerCli.ContainerCreate(ctx, &container.Config{
		Image:        repoName,
		Env:          []string{"PORT=8080"},
		ExposedPorts: map[nat.Port]struct{}{"8080/tcp": {}},
		Healthcheck:  nil,
	}, &container.HostConfig{
		PortBindings: nat.PortMap{
			"8080/tcp": []nat.PortBinding{{}},
		},
		AutoRemove: true,
	}, nil, containerName)
	c.assert.Nil(err)

	err = c.dockerCli.ContainerStart(ctx, ctr.ID, dockertypes.ContainerStartOptions{})
	c.assert.Nil(err)

	return TestContainer{
		testObject: c.testObject,
		dockerCli:  c.dockerCli,
		assert:     c.assert,
		name:       containerName,
	}
}

func (c TestContainerManager) RunDockerImageCombinedOutput(containerName, repoName string) string {
	c.testObject.Helper()
	ctx := context.Background()

	ctr, err := c.dockerCli.ContainerCreate(ctx, &container.Config{Image: repoName}, nil, nil, containerName)
	c.assert.Nil(err)

	bodyChan, errChan := c.dockerCli.ContainerWait(ctx, ctr.ID, container.WaitConditionNextExit)

	err = c.dockerCli.ContainerStart(ctx, ctr.ID, dockertypes.ContainerStartOptions{})
	c.assert.Nil(err)

	logs, err := c.dockerCli.ContainerLogs(ctx, ctr.ID, dockertypes.ContainerLogsOptions{
		ShowStdout: true,
		ShowStderr: true,
		Follow:     true,
	})
	c.assert.Nil(err)

	var output = new(bytes.Buffer)
	copyErr := make(chan error)
	go func() {
		_, err := stdcopy.StdCopy(output, output, logs)
		copyErr <- err
	}()

	select {
	case body := <-bodyChan:
		if body.StatusCode != 0 {
			c.testObject.Fatalf("failed with status code: %d", body.StatusCode)
		}
	case err := <-errChan:
		c.assert.Nil(err)
	case err := <-copyErr:
		c.assert.Nil(err)
	}

	return output.String()
}

func (c TestContainer) WaitForResponse(timeout time.Duration) string {
	c.testObject.Helper()

	appURI := fmt.Sprintf("http://localhost:%s", c.hostPort())

	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()
	timer := time.NewTimer(timeout)
	defer timer.Stop()

	for {
		select {
		case <-ticker.C:
			resp, err := h.HTTPGetE(appURI, map[string]string{})
			if err != nil {
				break
			}
			return resp
		case <-timer.C:
			c.testObject.Fatalf("timeout waiting for response: %v", timeout)
		}
	}
}

func (c TestContainer) Kill() {
	c.dockerCli.ContainerKill(context.Background(), c.name, "SIGKILL")
}

func (c TestContainer) Remove() {
	c.dockerCli.ContainerRemove(context.Background(), c.name, dockertypes.ContainerRemoveOptions{Force: true})
}

func (c TestContainer) hostPort() string {
	c.testObject.Helper()

	i, err := c.dockerCli.ContainerInspect(context.Background(), c.name)
	c.assert.Nil(err)
	for _, port := range i.NetworkSettings.Ports {
		for _, binding := range port {
			return binding.HostPort
		}
	}

	c.testObject.Fatalf("Failed to fetch host port for %s: no ports exposed", c.name)
	return ""
}
