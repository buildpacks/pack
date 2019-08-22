package build

import (
	"context"
	"fmt"
)

const (
	layersDir      = "/layers"
	appDir         = "/workspace"
	cacheDir       = "/cache"
	launchCacheDir = "/launch-cache"
	platformDir    = "/platform"
)

func (l *Lifecycle) Detect(ctx context.Context) error {
	detect, err := l.NewPhase(
		"detector",
		WithArgs(
			"-app", appDir,
			"-platform", platformDir,
		),
	)
	if err != nil {
		return err
	}
	defer detect.Cleanup()
	return detect.Run(ctx)
}

func (l *Lifecycle) Restore(ctx context.Context, useVolumeCache bool, cacheName string) error {
	var restore *Phase
	var err error

	if useVolumeCache {
		restore, err = l.NewPhase(
			"restorer",
			WithDaemonAccess(),
			WithArgs(
				"-path", cacheDir,
				"-layers", layersDir,
			),
			WithBinds(fmt.Sprintf("%s:%s", cacheName, cacheDir)),
		)
	} else {
		restore, err = l.NewPhase(
			"restorer",
			WithDaemonAccess(),
			WithArgs(
				"-image", cacheName,
				"-layers", layersDir,
			),
		)
	}

	if err != nil {
		return err
	}
	defer restore.Cleanup()
	return restore.Run(ctx)
}

func (l *Lifecycle) Analyze(ctx context.Context, repoName string, publish, clearCache bool) error {
	analyze, err := l.newAnalyze(repoName, publish, clearCache)
	if err != nil {
		return err
	}
	defer analyze.Cleanup()
	return analyze.Run(ctx)
}

func (l *Lifecycle) newAnalyze(repoName string, publish, clearCache bool) (*Phase, error) {
	args := []string{
		"-layers", layersDir,
		repoName,
	}
	if clearCache {
		args = prependArg("-skip-layers", args)
	}

	if publish {
		return l.NewPhase(
			"analyzer",
			WithRegistryAccess(repoName),
			WithArgs(args...),
		)
	} else {
		return l.NewPhase(
			"analyzer",
			WithDaemonAccess(),
			WithArgs(prependArg(
				"-daemon",
				args,
			)...),
		)
	}
}

func prependArg(arg string, args []string) []string {
	return append([]string{arg}, args...)
}

func (l *Lifecycle) Build(ctx context.Context) error {
	build, err := l.NewPhase(
		"builder",
		WithArgs(
			"-layers", layersDir,
			"-app", appDir,
			"-platform", platformDir,
		),
	)
	if err != nil {
		return err
	}
	defer build.Cleanup()
	return build.Run(ctx)
}

func (l *Lifecycle) Export(ctx context.Context, repoName string, runImage string, publish bool, launchCacheName string) error {
	export, err := l.newExport(repoName, runImage, publish, launchCacheName)
	if err != nil {
		return err
	}
	defer export.Cleanup()
	return export.Run(ctx)
}

func (l *Lifecycle) newExport(repoName, runImage string, publish bool, launchCacheName string) (*Phase, error) {
	if publish {
		return l.NewPhase(
			"exporter",
			WithRegistryAccess(repoName, runImage),
			WithArgs(
				"-image", runImage,
				"-layers", layersDir,
				"-app", appDir,
				repoName,
			),
		)
	} else {
		if launchCacheName != "" {
			return l.NewPhase(
				"exporter",
				WithDaemonAccess(),
				WithArgs(
					"-image", runImage,
					"-layers", layersDir,
					"-app", appDir,
					"-daemon",
					"-launch-cache", launchCacheDir,
					repoName,
				),
				WithBinds(fmt.Sprintf("%s:%s", launchCacheName, launchCacheDir)),
			)
		} else {
			return l.NewPhase(
				"exporter",
				WithDaemonAccess(),
				WithArgs(
					"-image", runImage,
					"-layers", layersDir,
					"-app", appDir,
					"-daemon",
					repoName,
				),
			)
		}
	}
}

func (l *Lifecycle) Cache(ctx context.Context, useVolumeCache bool, cacheName string) error {
	var cache *Phase
	var err error

	if useVolumeCache {
		cache, err = l.NewPhase(
			"cacher",
			WithDaemonAccess(),
			WithArgs(
				"-path", cacheDir,
				"-layers", layersDir,
			),
			WithBinds(fmt.Sprintf("%s:%s", cacheName, cacheDir)),
		)
	} else {
		cache, err = l.NewPhase(
			"cacher",
			WithDaemonAccess(),
			WithArgs(
				"-image", cacheName,
				"-layers", layersDir,
			),
		)
	}

	if err != nil {
		return err
	}
	defer cache.Cleanup()
	return cache.Run(ctx)
}
