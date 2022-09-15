package cache

import (
	"context"
	"os"

	"github.com/docker/docker/client"
)

type BindCache struct {
	docker client.CommonAPIClient
	bind   string
}

func NewBindCache(cacheType CacheInfo, dockerClient client.CommonAPIClient) *BindCache {
	return &BindCache{
		bind:   cacheType.Source,
		docker: dockerClient,
	}
}

func (c *BindCache) Name() string {
	return c.bind
}

func (c *BindCache) Clear(ctx context.Context) error {
	err := os.RemoveAll(c.bind)
	if err != nil {
		return err
	}
	return nil
}

func (c *BindCache) Type() Type {
	return Bind
}
