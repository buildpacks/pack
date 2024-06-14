package client

import (
	"context"
	"fmt"

	"github.com/docker/docker/api/types"
)

func (c *Client) Detect(ctx context.Context, opts BuildOptions) error {
	lifecycleOpts, err := c.ResolveLifecycleOptions(ctx, opts)
	if err != nil {
		return err
	}

	defer c.docker.ImageRemove(context.Background(), lifecycleOpts.LifecycleImage, types.ImageRemoveOptions{Force: true})
	defer c.docker.ImageRemove(context.Background(), lifecycleOpts.Builder.Name(), types.ImageRemoveOptions{Force: true})

	if err = c.lifecycleExecutor.Detect(ctx, *lifecycleOpts); err != nil {
		return fmt.Errorf("executing detect: %w", err)
	}
	// Log / Save to disk, the final detected group
	return nil
}
