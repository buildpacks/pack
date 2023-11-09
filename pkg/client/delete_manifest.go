package client

import (
	"context"
	// "fmt"
	// "strings"
)

// DeleteManifest implements commands.PackClient.
func (c *Client) DeleteManifest(ctx context.Context, names []string) error {
	return c.runtime.RemoveManifests(ctx, names)
}