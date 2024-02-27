package client

import (
	"context"

	scafall "github.com/buildpacks/scafall/pkg"
)

type CreateBuildpackOptions struct {
	// URL of git repository containing the project template
	Template string

	// Subdirectory within the repository containing the project template
	SubPath string

	// arguments to provide to the project template
	Arguments map[string]string
}

func (c *Client) CreateBuildpack(ctx context.Context, opts CreateBuildpackOptions) error {
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
