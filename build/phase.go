package build

import (
	"context"
	"fmt"
	"os"
	"sync"

	"github.com/buildpack/lifecycle/image/auth"

	"github.com/buildpack/pack/archive"
	"github.com/buildpack/pack/logging"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/pkg/errors"
)

type Phase struct {
	name     string
	logger   *logging.Logger
	docker   Docker
	ctrConf  *container.Config
	hostConf *container.HostConfig
	ctr      container.ContainerCreateCreatedBody
	uid, gid int
	appDir   string
	appOnce  *sync.Once
}

func (l *Lifecycle) NewPhase(name string, ops ...func(*Phase) (*Phase, error)) (*Phase, error) {
	ctrConf := &container.Config{
		Image:  l.BuilderImage,
		Labels: map[string]string{"author": "pack"},
	}
	hostConf := &container.HostConfig{
		Binds: []string{
			fmt.Sprintf("%s:%s:", l.LayersVolume, layersDir),
			fmt.Sprintf("%s:%s:", l.AppVolume, appDir),
		},
	}
	ctrConf.Cmd = []string{"/lifecycle/" + name}
	phase := &Phase{
		ctrConf:  ctrConf,
		hostConf: hostConf,
		name:     name,
		docker:   l.Docker,
		logger:   l.Logger,
		uid:      l.uid,
		gid:      l.gid,
		appDir:   l.appDir,
		appOnce:  l.appOnce,
	}
	var err error
	for _, op := range ops {
		phase, err = op(phase)
		if err != nil {
			return nil, errors.Wrapf(err, "create %s phase", name)
		}
	}
	return phase, nil
}

func WithArgs(args ...string) func(*Phase) (*Phase, error) {
	return func(phase *Phase) (*Phase, error) {
		phase.ctrConf.Cmd = append(phase.ctrConf.Cmd, args...)
		return phase, nil
	}
}

func WithDaemonAccess() func(*Phase) (*Phase, error) {
	return func(phase *Phase) (*Phase, error) {
		phase.ctrConf.User = "root"

		if dockerVolumeName, ok := os.LookupEnv(`PACK_DOCKER_CERT_VOLUME`); ok {
			phase.ctrConf.Env = []string{
				fmt.Sprintf(`DOCKER_HOST=%s`, os.Getenv(`DOCKER_HOST`)),
				fmt.Sprintf(`DOCKER_TLS_VERIFY=%s`, os.Getenv(`DOCKER_TLS_VERIFY`)),
				fmt.Sprintf(`DOCKER_CERT_PATH=%s`, `/pack-docker-cert-path`),
			}
			phase.hostConf.Binds = append(phase.hostConf.Binds, fmt.Sprintf(`%s:/pack-docker-cert-path`, dockerVolumeName))
		}

		phase.hostConf.Binds = append(phase.hostConf.Binds, "/var/run/docker.sock:/var/run/docker.sock")
		return phase, nil
	}
}

func WithRegistryAccess(repos ...string) func(*Phase) (*Phase, error) {
	return func(phase *Phase) (*Phase, error) {
		authHeader, err := auth.BuildEnvVar(authn.DefaultKeychain, repos...)
		if err != nil {
			return nil, err
		}
		phase.ctrConf.Env = []string{fmt.Sprintf(`CNB_REGISTRY_AUTH=%s`, authHeader)}
		phase.hostConf.NetworkMode = "host"
		return phase, nil
	}
}

func (p *Phase) Run(context context.Context) error {
	var err error
	p.ctr, err = p.docker.ContainerCreate(context, p.ctrConf, p.hostConf, nil, "")
	if err != nil {
		return errors.Wrapf(err, "failed to create '%s' container", p.name)
	}
	p.appOnce.Do(func() {
		appReader, _ := archive.CreateTarReader(p.appDir, appDir, p.uid, p.gid)
		if err := p.docker.CopyToContainer(context, p.ctr.ID, "/", appReader, types.CopyToContainerOptions{}); err != nil {
			err = errors.Wrapf(err, "failed to copy files to '%s' container", p.name)
		}
	})
	if err != nil {
		return errors.Wrapf(err, "run %s container", p.name)
	}
	return p.docker.RunContainer(
		context,
		p.ctr.ID,
		p.logger.VerboseWriter().WithPrefix(p.name),
		p.logger.VerboseErrorWriter().WithPrefix(p.name),
	)
}

func (p *Phase) Cleanup() error {
	return p.docker.ContainerRemove(context.Background(), p.ctr.ID, types.ContainerRemoveOptions{Force: true})
}
