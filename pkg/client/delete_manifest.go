package client

import (
	"context"
	"fmt"
	"strings"
)

// DeleteManifest implements commands.PackClient.
func (c *Client) DeleteManifest(ctx context.Context, names []string) error {
	rmiReports, rmiErrors := c.runtime.RemoveManifests(ctx, names)
	for _, r := range rmiReports {
		for _, u := range r.Untagged {
			fmt.Printf("untagged: %s\n", u)
		}
	}

	for _, r := range rmiReports {
		if r.Removed {
			fmt.Printf("%s\n", r.ID)
		}
	}
	var errors []string
	for _, err := range rmiErrors {
		errors = append(errors, err.Error())
	}

	if len(errors) == 0 {
		return nil
	}

	return fmt.Errorf(strings.Join(errors, "\n"))
}
