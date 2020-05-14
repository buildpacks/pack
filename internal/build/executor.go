package build

import (
	"context"
	"github.com/Masterminds/semver"
	"github.com/buildpacks/pack/internal/cache"
	"github.com/buildpacks/pack/internal/style"
	"github.com/buildpacks/pack/logging"
	dockercli "github.com/docker/docker/client"
	"github.com/pkg/errors"
)

type Creator interface {
	Create(ctx context.Context, publish, clearCache bool, runImage, launchCacheName, cacheName, repoName, networkMode string, phaseFactory PhaseFactory) error
	Detect(ctx context.Context, networkMode string, volumes []string, phaseFactory PhaseFactory) error
	Analyze(ctx context.Context, repoName, cacheName, networkMode string, publish, clearCache bool, phaseFactory PhaseFactory) error
	Restore(ctx context.Context, cacheName, networkMode string, phaseFactory PhaseFactory) error
	Build(ctx context.Context, networkMode string, volumes []string, phaseFactory PhaseFactory) error
	Export(ctx context.Context, repoName string, runImage string, publish bool, launchCacheName, cacheName, networkMode string, phaseFactory PhaseFactory) error
}

type Executor struct {}

func (e *Executor) Execute(
	ctx context.Context,
	opts LifecycleOptions,
	l Creator,
	platformAPIVersion string, // TODO: should this be a method on the Lifecycle?
	phaseFactory *DefaultPhaseFactory,
	docker dockercli.CommonAPIClient,
	logger logging.Logger,
) error {
	buildCache := cache.NewVolumeCache(opts.Image, "build", docker)
	logger.Debugf("Using build cache volume %s", style.Symbol(buildCache.Name()))
	if opts.ClearCache {
		if err := buildCache.Clear(ctx); err != nil {
			return errors.Wrap(err, "clearing build cache")
		}
		logger.Debugf("Build cache %s cleared", style.Symbol(buildCache.Name()))
	}

	launchCache := cache.NewVolumeCache(opts.Image, "launch", docker)

	if semver.MustParse(platformAPIVersion).LessThan(semver.MustParse("0.3")) || (opts.Publish && !opts.TrustBuilder) { // TODO: change the boundary to lifecycle version 0.7.5
		logger.Info(style.Step("DETECTING"))
		if err := l.Detect(ctx, opts.Network, opts.Volumes, phaseFactory); err != nil {
			return err
		}

		logger.Info(style.Step("ANALYZING"))
		if err := l.Analyze(ctx, opts.Image.Name(), buildCache.Name(), opts.Network, opts.Publish, opts.ClearCache, phaseFactory); err != nil {
			return err
		}

		logger.Info(style.Step("RESTORING"))
		if opts.ClearCache {
			logger.Info("Skipping 'restore' due to clearing cache")
		} else if err := l.Restore(ctx, buildCache.Name(), opts.Network, phaseFactory); err != nil {
			return err
		}

		logger.Info(style.Step("BUILDING"))

		if err := l.Build(ctx, opts.Network, opts.Volumes, phaseFactory); err != nil {
			return err
		}

		logger.Info(style.Step("EXPORTING"))
		return l.Export(ctx, opts.Image.Name(), opts.RunImage, opts.Publish, launchCache.Name(), buildCache.Name(), opts.Network, phaseFactory)
	}

	logger.Info(style.Step("CREATING"))
	return l.Create(ctx, opts.Publish, opts.ClearCache, opts.RunImage, launchCache.Name(), buildCache.Name(), opts.Image.Name(), opts.Network, phaseFactory)
}
