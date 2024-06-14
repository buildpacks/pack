package client

import (
	"context"
	"fmt"
)

func (c *Client) Detect(ctx context.Context, opts BuildOptions) error {
	lifecycleOpts, err := c.ResolveLifecycleOptions(ctx, opts)
	if err != nil {
		return err
	}

	// TODO: Cleanup

	if err = c.lifecycleExecutor.Detect(ctx, *lifecycleOpts); err != nil {
		return fmt.Errorf("executing detect: %w", err)
	}
	// Log / Save to disk, the final detected group
	return nil
}
