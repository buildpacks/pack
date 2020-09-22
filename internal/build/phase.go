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
	"github.com/buildpacks/pack/internal/style"
)

type Phase struct {
	name          string
	infoWriter    io.Writer
	errorWriter   io.Writer
	docker        client.CommonAPIClient
	ctrConf       *dcontainer.Config
	hostConf      *dcontainer.HostConfig
	createdCtrIDs []string
	uid, gid      int
	appPath       string
	containerOps  []ContainerOperation
	intercept     string
	fileFilter    func(string) bool
}

func (p *Phase) Run(ctx context.Context) error {
	if p.intercept != "" {
		if err := p.attemptToIntercept(ctx); err == nil {
			return nil
		} else {
			_, _ = fmt.Fprintf(p.errorWriter, "Failed to start intercepted container: %s\n", err.Error())
			_, _ = fmt.Fprintln(p.infoWriter, "Phase will run without interception")
		}
	}

	ctrID, err := p.createContainer(ctx, p.ctrConf)
	if err != nil {
		return errors.Wrapf(err, "failed to create '%s' container", p.name)
	}

	for _, containerOp := range p.containerOps {
		if err := containerOp(p.docker, ctx, ctrID, p.infoWriter, p.errorWriter); err != nil {
			return err
		}
	}

	return container.Run(
		ctx,
		p.docker,
		ctrID,
		p.infoWriter,
		p.errorWriter,
	)
}

func (p *Phase) attemptToIntercept(ctx context.Context) error {
	originalCmd := p.ctrConf.Cmd

	ctrConf := *p.ctrConf
	ctrConf.Cmd = []string{p.intercept}
	ctrConf.AttachStdin = true
	ctrConf.AttachStdout = true
	ctrConf.AttachStderr = true
	ctrConf.Tty = true
	ctrConf.OpenStdin = true

	ctrID, err := p.createContainer(ctx, &ctrConf)
	if err != nil {
		return err
	}

	_, _ = fmt.Fprintf(p.infoWriter, `Attempting to intercept...
-----------
To continue to the next phase type: %s
To manually run the phase type:
%s
-----------
`, style.Symbol("exit"), style.Symbol(strings.Join(originalCmd, " ")))

	return container.Start(ctx, p.docker, ctrID, types.ContainerStartOptions{})
}

func (p *Phase) createContainer(ctx context.Context, ctrConf *dcontainer.Config) (ctrID string, err error) {
	ctr, err := p.docker.ContainerCreate(ctx, ctrConf, p.hostConf, nil, "")
	if err != nil {
		return "", errors.Wrapf(err, "failed to create '%s' container", p.name)
	}

	p.createdCtrIDs = append(p.createdCtrIDs, ctr.ID)

	for _, containerOp := range p.containerOps {
		if err := containerOp(p.docker, ctx, ctr.ID, p.infoWriter, p.errorWriter); err != nil {
			return "", err
		}
	}

	return ctr.ID, nil
}

func (p *Phase) Cleanup() error {
	var err error
	for _, ctrID := range p.createdCtrIDs {
		err = p.docker.ContainerRemove(context.Background(), ctrID, types.ContainerRemoveOptions{Force: true})
	}
	return err
}
