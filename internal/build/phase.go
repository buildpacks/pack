package build

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/docker/docker/api/types"
	dcontainer "github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"
	"github.com/pkg/errors"

	"github.com/buildpacks/pack/internal/container"
)

type Phase struct {
	name         string
	infoWriter   io.Writer
	errorWriter  io.Writer
	docker       client.CommonAPIClient
	ctrConf      *dcontainer.Config
	hostConf     *dcontainer.HostConfig
	ctr          dcontainer.ContainerCreateCreatedBody
	uid, gid     int
	appPath      string
	containerOps []ContainerOperation
	fileFilter   func(string) bool
}

func (p *Phase) Run(ctx context.Context) error {
	var (
		err error
		// TODO: Pass this as a flag via cmd
		intercept   = true
		originalCmd = p.ctrConf.Cmd
	)

	if intercept {
		p.ctrConf.Cmd = []string{"/bin/sh"}
		p.ctrConf.AttachStdin = true
		p.ctrConf.AttachStdout = true
		p.ctrConf.AttachStderr = true
		p.ctrConf.Tty = true
		p.ctrConf.OpenStdin = true
	}

	p.ctr, err = p.docker.ContainerCreate(ctx, p.ctrConf, p.hostConf, nil, "")
	if err != nil {
		return errors.Wrapf(err, "failed to create '%s' container", p.name)
	}

	for _, containerOp := range p.containerOps {
		if err := containerOp(p.docker, ctx, p.ctr.ID); err != nil {
			return err
		}
	}

	if intercept {
		_, _ = fmt.Fprint(p.infoWriter, "Intercepting...")
		_, _ = fmt.Fprintf(p.infoWriter, `-----------
To continue to the next phase type: exit
To manually run the phase type:
%s
-----------
`, strings.Join(originalCmd, " "))

		err = container.Start(ctx, p.docker, p.ctr.ID, types.ContainerStartOptions{})
		if err != nil {
			return errors.Wrapf(err, "start container")
		}
	} else {
		err = container.Run(
			ctx,
			p.docker,
			p.ctr.ID,
			p.infoWriter,
			p.errorWriter,
		)
		if err != nil {
			return errors.Wrapf(err, "start container")
		}
	}

	return nil
}

func (p *Phase) Cleanup() error {
	return p.docker.ContainerRemove(context.Background(), p.ctr.ID, types.ContainerRemoveOptions{Force: true})
}
