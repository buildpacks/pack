package build

import (
	"context"
)

const (
	layersDir     = "/layers"
	buildpacksDir = "/buildpacks"
	platformDir   = "/platform"
	orderPath     = "/buildpacks/order.toml"
	groupPath     = `/layers/group.toml`
	planPath      = "/layers/plan.toml"
	appDir        = "/workspace"
)

func (l *Lifecycle) Detect(ctx context.Context) error {
	detect, err := l.NewPhase(
		"detector",
		WithArgs(
			"-buildpacks", buildpacksDir,
			"-order", orderPath,
			"-group", groupPath,
			"-plan", planPath,
			"-app", appDir,
		),
	)
	if err != nil {
		return err
	}
	defer detect.Cleanup()
	return detect.Run(ctx)
}

func (l *Lifecycle) Restore(ctx context.Context, cacheImage string) error {
	restore, err := l.NewPhase(
		"restorer",
		WithDaemonAccess(),
		WithArgs(
			"-image", cacheImage,
			"-group", groupPath,
			"-layers", layersDir,
		),
	)
	if err != nil {
		return err
	}
	defer restore.Cleanup()
	return restore.Run(ctx)
}

func (l *Lifecycle) Analyze(ctx context.Context, repoName string, publish bool) error {
	analyze, err := l.newAnalyze(repoName, publish)
	if err != nil {
		return err
	}
	defer analyze.Cleanup()
	return analyze.Run(ctx)
}

func (l *Lifecycle) newAnalyze(repoName string, publish bool) (*Phase, error) {
	if publish {
		return l.NewPhase(
			"analyzer",
			WithRegistryAccess(repoName),
			WithArgs(
				"-layers", layersDir,
				"-group", groupPath,
				repoName,
			),
		)
	} else {
		return l.NewPhase(
			"analyzer",
			WithDaemonAccess(),
			WithArgs(
				"-layers", layersDir,
				"-group", groupPath,
				"-daemon",
				repoName,
			),
		)
	}
}

func (l *Lifecycle) Build(ctx context.Context) error {
	build, err := l.NewPhase(
		"builder",
		WithArgs(
			"-buildpacks", buildpacksDir,
			"-layers", layersDir,
			"-app", appDir,
			"-group", groupPath,
			"-plan", planPath,
			"-platform", platformDir,
		),
	)
	if err != nil {
		return err
	}
	defer build.Cleanup()
	return build.Run(ctx)
}

func (l *Lifecycle) Export(ctx context.Context, repoName string, runImage string, publish bool) error {
	export, err := l.newExport(repoName, runImage, publish)
	if err != nil {
		return err
	}
	defer export.Cleanup()
	return export.Run(ctx)
}

func (l *Lifecycle) newExport(repoName, runImage string, publish bool) (*Phase, error) {
	if publish {
		return l.NewPhase(
			"exporter",
			WithRegistryAccess(repoName, runImage),
			WithArgs(
				"-image", runImage,
				"-layers", layersDir,
				"-app", appDir,
				"-group", groupPath,
				repoName,
			),
		)
	} else {
		return l.NewPhase(
			"exporter",
			WithDaemonAccess(),
			WithArgs(
				"-image", runImage,
				"-layers", layersDir,
				"-app", appDir,
				"-group", groupPath,
				"-daemon",
				repoName,
			),
		)
	}
}

func (l *Lifecycle) Cache(ctx context.Context, cacheImage string) error {
	cache, err := l.NewPhase(
		"cacher",
		WithDaemonAccess(),
		WithArgs(
			"-image", cacheImage,
			"-group", groupPath,
			"-layers", layersDir,
		),
	)
	if err != nil {
		return err
	}
	defer cache.Cleanup()
	return cache.Run(ctx)
}
