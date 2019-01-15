package cache

import (
	"context"
	"crypto/md5"
	"fmt"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/filters"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/pkg/errors"

	"github.com/buildpack/pack/docker"
)

type Cache struct {
	Docker *docker.Client
	Volume string
}

func New(repoName string) (*Cache, error) {
	ref, err := name.ParseReference(repoName, name.WeakValidation)
	if err != nil {
		return nil, errors.Wrap(err, "bad image identifier")
	}
	dockerCli, err := docker.New()
	if err != nil {
		return nil, errors.Wrap(err, "failed to make new docker client")
	}
	return &Cache{
		Volume: fmt.Sprintf("pack-cache-%x", md5.Sum([]byte(ref.String()))),
		Docker: dockerCli,
	}, nil
}

func (c *Cache) Clear() error {
	allContainers, err := c.Docker.ContainerList(context.Background(), types.ContainerListOptions{
		All: true,
		Filters: filters.NewArgs(filters.KeyValuePair{
			Key:   "volume",
			Value: c.Volume,
		}),
	})
	if err != nil {
		return err
	}
	for _, ctr := range allContainers {
		if author, ok := ctr.Labels["author"]; ok && author == "pack" {
			c.Docker.ContainerRemove(context.Background(), ctr.ID, types.ContainerRemoveOptions{
				Force: true,
			})
		} else {
			return fmt.Errorf("volume in use by the container '%s' not created by pack", ctr.ID)
		}
	}

	err = c.Docker.VolumeRemove(context.Background(), c.Volume, true)
	if err != nil {
		return err
	}
	return nil
}
