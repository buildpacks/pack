package cache

import (
	"context"
	"crypto/md5"
	"fmt"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/filters"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/pkg/errors"

	"github.com/buildpack/pack/containers"
)

type Cache struct {
	docker Docker
	volume string
}

type Docker interface {
	VolumeRemove(ctx context.Context, volumeID string, force bool) error
	ContainerList(ctx context.Context, options types.ContainerListOptions) ([]types.Container, error)
	ContainerRemove(ctx context.Context, containerID string, options types.ContainerRemoveOptions) error
}

func New(repoName string, dockerClient Docker) (*Cache, error) {
	ref, err := name.ParseReference(repoName, name.WeakValidation)
	if err != nil {
		return nil, errors.Wrap(err, "bad image identifier")
	}
	return &Cache{
		volume: fmt.Sprintf("pack-cache-%x", md5.Sum([]byte(ref.String()))),
		docker: dockerClient,
	}, nil
}

func (c *Cache) Volume() string {
	return c.volume
}

func (c *Cache) Clear(ctx context.Context) error {
	allContainers, err := c.docker.ContainerList(ctx, types.ContainerListOptions{
		All: true,
		Filters: filters.NewArgs(filters.KeyValuePair{
			Key:   "volume",
			Value: c.volume,
		}),
	})
	if err != nil {
		return err
	}
	for _, ctr := range allContainers {
		if author, ok := ctr.Labels["author"]; ok && author == "pack" {
			_ = containers.Remove(c.docker, ctr.ID)
		} else {
			return fmt.Errorf("volume in use by the container '%s' not created by pack", ctr.ID)
		}
	}

	err = c.docker.VolumeRemove(ctx, c.volume, true)
	if err != nil {
		return err
	}
	return nil
}
