package build

import (
	"context"
	"io"

	"github.com/docker/docker/api/types"
	containertypes "github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/image"
	networktypes "github.com/docker/docker/api/types/network"
	dockerClient "github.com/docker/docker/client"
	specs "github.com/opencontainers/image-spec/specs-go/v1"
)

//go:generate mockgen -package mockdocker -destination ./mockdocker/mockDockerClient.go github.com/buildpacks/pack/internal/build DockerClient

type DockerClient interface {
	ImageRemove(ctx context.Context, image string, options types.ImageRemoveOptions) ([]image.DeleteResponse, error)
	VolumeRemove(ctx context.Context, volumeID string, force bool) error
	ContainerWait(ctx context.Context, container string, condition containertypes.WaitCondition) (<-chan containertypes.WaitResponse, <-chan error)
	ContainerAttach(ctx context.Context, container string, options containertypes.AttachOptions) (types.HijackedResponse, error)
	ContainerStart(ctx context.Context, container string, options containertypes.StartOptions) error
	ContainerCreate(ctx context.Context, config *containertypes.Config, hostConfig *containertypes.HostConfig, networkingConfig *networktypes.NetworkingConfig, platform *specs.Platform, containerName string) (containertypes.CreateResponse, error)
	CopyFromContainer(ctx context.Context, container, srcPath string) (io.ReadCloser, types.ContainerPathStat, error)
	ContainerInspect(ctx context.Context, container string) (types.ContainerJSON, error)
	ContainerRemove(ctx context.Context, container string, options containertypes.RemoveOptions) error
	CopyToContainer(ctx context.Context, container, path string, content io.Reader, options types.CopyToContainerOptions) error
	ImageBuild(ctx context.Context, buildContext io.Reader, options types.ImageBuildOptions) (types.ImageBuildResponse, error)
	ImageInspectWithRaw(ctx context.Context, image string) (types.ImageInspect, []byte, error)
	ImageSave(ctx context.Context, imageIDs []string) (io.ReadCloser, error)
}

var _ DockerClient = dockerClient.CommonAPIClient(nil)
