package pack

import (
	"context"
	"io"

	"github.com/buildpack/lifecycle/image"
	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/network"
	"github.com/google/go-containerregistry/pkg/v1"
)

//go:generate mockgen -package mocks -destination mocks/docker.go github.com/buildpack/pack Docker
type Docker interface {
	RunContainer(ctx context.Context, id string, stdout io.Writer, stderr io.Writer) error
	VolumeRemove(ctx context.Context, volumeID string, force bool) error
	ContainerCreate(ctx context.Context, config *container.Config, hostConfig *container.HostConfig, networkingConfig *network.NetworkingConfig, containerName string) (container.ContainerCreateCreatedBody, error)
	ContainerRemove(ctx context.Context, containerID string, options types.ContainerRemoveOptions) error
	ContainerList(ctx context.Context, options types.ContainerListOptions) ([]types.Container, error)
	CopyToContainer(ctx context.Context, containerID, dstPath string, content io.Reader, options types.CopyToContainerOptions) error
	CopyFromContainer(ctx context.Context, containerID, srcPath string) (io.ReadCloser, types.ContainerPathStat, error)
	ImageBuild(ctx context.Context, buildContext io.Reader, options types.ImageBuildOptions) (types.ImageBuildResponse, error)
	ImageInspectWithRaw(ctx context.Context, imageID string) (types.ImageInspect, []byte, error)
	ImageRemove(ctx context.Context, imageID string, options types.ImageRemoveOptions) ([]types.ImageDeleteResponseItem, error)
	PullImage(ctx context.Context, imageID string, stdout io.Writer) error
}

//go:generate mockgen -package mocks -destination mocks/task.go github.com/buildpack/pack Task
type Task interface {
	Run(context context.Context) error
}

//go:generate mockgen -package mocks -destination mocks/writablestore.go github.com/buildpack/pack WritableStore
type WritableStore interface {
	Write(image v1.Image) error
}

//go:generate mockgen -package mocks -destination mocks/image_factory.go github.com/buildpack/pack ImageFactory
type ImageFactory interface {
	NewLocal(string) (image.Image, error)
	NewRemote(string) (image.Image, error)
}

//go:generate mockgen -package mocks -destination mocks/fetcher.go github.com/buildpack/pack Fetcher
type Fetcher interface {
	FetchUpdatedLocalImage(context.Context, string, io.Writer) (image.Image, error)
	FetchLocalImage(string) (image.Image, error)
	FetchRemoteImage(string) (image.Image, error)
}
