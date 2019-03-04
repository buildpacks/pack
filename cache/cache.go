package cache

import (
	"context"
	"crypto/md5"
	"fmt"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/client"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/pkg/errors"

	"github.com/buildpack/pack/docker"
)

type Cache struct {
	docker *docker.Client
	image  string
}

//type Docker interface {
//	VolumeRemove(ctx context.Context, volumeID string, force bool) error
//	ContainerList(ctx context.Context, options types.ContainerListOptions) ([]types.Container, error)
//	ContainerRemove(ctx context.Context, containerID string, options types.ContainerRemoveOptions) error
//}

func New(repoName string, dockerClient *docker.Client) (*Cache, error) {
	ref, err := name.ParseReference(repoName, name.WeakValidation)
	if err != nil {
		return nil, errors.Wrap(err, "bad image identifier")
	}
	return &Cache{
		image:  fmt.Sprintf("pack-cache-%x", md5.Sum([]byte(ref.String()))),
		docker: dockerClient,
	}, nil
}

func (c *Cache) Image() string {
	return c.image
}

func (c *Cache) Clear(ctx context.Context) error {
	_, err := c.docker.ImageRemove(ctx, c.Image(), types.ImageRemoveOptions{
		Force: true,
	})
	if err != nil && !client.IsErrNotFound(err) {
		return err
	}
	return nil
}
