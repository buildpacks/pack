package cache

import (
	"context"
	"crypto/sha256"
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

func New(repoName string, dockerClient *docker.Client) (*Cache, error) {
	ref, err := name.ParseReference(repoName, name.WeakValidation)
	if err != nil {
		return nil, errors.Wrap(err, "bad image identifier")
	}

	sum := sha256.Sum256([]byte(ref.String()))

	return &Cache{
		image:  fmt.Sprintf("pack-cache-%x", sum[:6]),
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