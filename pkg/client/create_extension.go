package client

import (
	"context"

	scafall "github.com/buildpacks/scafall/pkg"
)

type CreateExtensionOptions struct {
	Arguments map[string]string // arguments to provide to the project template
	Template  string            // URL of git repository containing the project template
	SubPath   string            // Subdirectory within the repository containing the project template
}

func (c *Client) CreateExtension(ctx context.Context, opts CreateExtensionOptions) error {
	ops := []scafall.Option{scafall.WithSubPath(opts.SubPath)}
	if opts.Arguments != nil {
		ops = append(ops, scafall.WithArguments(opts.Arguments))
	}
	s, err := scafall.NewScafall(opts.Template, ops...)
	if err != nil {
		return err
	}
	err = s.Scaffold()
	if err != nil {
		return err
	}
	return nil
}
