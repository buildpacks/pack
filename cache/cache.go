package cache

import (
	"context"
	"crypto/sha256"
	"fmt"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/client"
	"github.com/google/go-containerregistry/pkg/name"
)

type Cache struct {
	docker *client.Client
	image  string
}

func New(imageRef name.Reference, dockerClient *client.Client) *Cache {
	sum := sha256.Sum256([]byte(imageRef.String()))
	return &Cache{
		image:  fmt.Sprintf("pack-cache-%x", sum[:6]),
		docker: dockerClient,
	}
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
