package testhelpers

import (
	"context"
	"fmt"
	"testing"
	"time"

	dockertypes "github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/go-connections/nat"
)

type DockerRegistry struct {
	Port string
	Name string
}

func NewDockerRegistry() *DockerRegistry {
	return &DockerRegistry{}
}

func (registry *DockerRegistry) Start(t *testing.T) {
	t.Log("run registry")
	t.Helper()
	registry.Name = "test-registry-" + RandString(10)

	AssertNil(t, PullImage(DockerCli(t), "registry:2"))
	ctx := context.Background()
	ctr, err := DockerCli(t).ContainerCreate(ctx, &container.Config{
		Image: "registry:2",
	}, &container.HostConfig{
		AutoRemove: true,
		PortBindings: nat.PortMap{
			"5000/tcp": []nat.PortBinding{{}},
		},
	}, nil, registry.Name)
	AssertNil(t, err)
	err = DockerCli(t).ContainerStart(ctx, ctr.ID, dockertypes.ContainerStartOptions{})
	AssertNil(t, err)

	inspect, err := DockerCli(t).ContainerInspect(ctx, ctr.ID)
	AssertNil(t, err)
	registry.Port = inspect.NetworkSettings.Ports["5000/tcp"][0].HostPort

	Eventually(t, func() bool {
		txt, err := HttpGetE(fmt.Sprintf("http://localhost:%s/v2/", registry.Port))
		return err == nil && txt != ""
	}, 100*time.Millisecond, 10*time.Second)
}

func (registry *DockerRegistry) Stop(t *testing.T) {
	t.Log("stop registry")
	t.Helper()
	if registry.Name != "" {
		DockerCli(t).ContainerKill(context.Background(), registry.Name, "SIGKILL")
		DockerCli(t).ContainerRemove(context.TODO(), registry.Name, dockertypes.ContainerRemoveOptions{Force: true})
	}
}
