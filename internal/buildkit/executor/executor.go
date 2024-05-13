package executor

import (
	"context"
	"fmt"
	"os"

	"github.com/google/go-containerregistry/pkg/name"
	"github.com/moby/buildkit/client"
	"github.com/pkg/errors"

	"github.com/buildpacks/pack/internal/build"
	"github.com/buildpacks/pack/internal/buildkit/lifecycle"
	"github.com/buildpacks/pack/internal/style"
	"github.com/buildpacks/pack/pkg/cache"
)

func (l LifecycleExecutor) Execute(ctx context.Context, opts build.LifecycleOptions) error {
	tmpDir, err := os.MkdirTemp("", "pack.tmp")
	if err != nil {
		return err
	}

	exec, err := lifecycle.NewLifecycleExecution(l.logger, l.state, l.targets, tmpDir, opts)
	if err != nil {
		return err
	}

	client, err := client.New(ctx, "")
	if err != nil {
		return err
	}

	defer client.Close()

	var buildCache build.Cache
	if opts.CacheImage != "" || (opts.Cache.Build.Format == cache.CacheImage) {
		cacheImageName := opts.CacheImage
		if cacheImageName == "" {
			cacheImageName = opts.Cache.Build.Source
		}
		cacheImage, err := name.ParseReference(cacheImageName, name.WeakValidation)
		if err != nil {
			return fmt.Errorf("invalid cache image name: %s", err)
		}
		buildCache = cache.NewImageCache(cacheImage, l.dockerClient)
	} else {
		switch opts.Cache.Build.Format {
		case cache.CacheVolume:
			buildCache = cache.NewVolumeCache(opts.Image, opts.Cache.Build, "build", l.dockerClient)
			l.logger.Debugf("Using build cache volume %s", style.Symbol(buildCache.Name()))
		case cache.CacheBind:
			buildCache = cache.NewBindCache(opts.Cache.Build, l.dockerClient)
			l.logger.Debugf("Using build cache dir %s", style.Symbol(buildCache.Name()))
		}
	}

	if opts.ClearCache {
		if err := buildCache.Clear(ctx); err != nil {
			return errors.Wrap(err, "clearing build cache")
		}
		l.logger.Debugf("Build cache %s cleared", style.Symbol(buildCache.Name()))
	}

	launchCache := cache.NewVolumeCache(opts.Image, opts.Cache.Launch, "launch", l.dockerClient)
	fmt.Printf("volumes: build=%s \n launch=%s \n\n\n", buildCache.Name(), launchCache.Name())
	if !opts.Interactive {
		return exec.Create(ctx, client, buildCache, launchCache)
	}

	return opts.Termui.Run(func() {
		exec.Create(ctx, client, buildCache, launchCache)
	})
}
