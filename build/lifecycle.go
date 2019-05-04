package build

import (
	"context"
	"math/rand"
	"sync"
	"time"

	"github.com/docker/docker/client"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/pkg/errors"

	"github.com/buildpack/pack/builder"
	"github.com/buildpack/pack/buildpack"
	"github.com/buildpack/pack/cache"
	"github.com/buildpack/pack/logging"
	"github.com/buildpack/pack/style"
)

type Lifecycle struct {
	Builder      *builder.Builder
	logger       *logging.Logger
	docker       *client.Client
	LayersVolume string
	AppVolume    string
	appDir       string
	appOnce      *sync.Once
}

type LifecycleConfig struct {
	BuilderImage string
	Logger       *logging.Logger
	Env          map[string]string
	Buildpacks   []string
	AppDir       string
	BPFetcher    *buildpack.Fetcher
}

func init() {
	rand.Seed(time.Now().UTC().UnixNano())
}

func NewLifecycle(docker *client.Client, logger *logging.Logger) *Lifecycle {
	return &Lifecycle{logger: logger, docker: docker}
}

type LifecycleOptions struct {
	AppDir     string
	Image      name.Reference
	Builder    *builder.Builder
	RunImage   string
	ClearCache bool
	Publish    bool
}

func (l *Lifecycle) Execute(ctx context.Context, opts LifecycleOptions) error {
	cacheImage := cache.New(opts.Image, l.docker)
	l.logger.Verbose("Using cache image %s", style.Symbol(cacheImage.Image()))
	if opts.ClearCache {
		if err := cacheImage.Clear(ctx); err != nil {
			return errors.Wrap(err, "clearing cache")
		}
		l.logger.Verbose("Cache image %s cleared", style.Symbol(cacheImage.Image()))
	}
	l.Setup(opts.AppDir, opts.Builder)
	defer l.Cleanup()

	if lifecycleVersion := l.Builder.GetLifecycleVersion(); lifecycleVersion == "" {
		l.logger.Verbose("Warning: lifecycle version unknown")
	}else {
		l.logger.Verbose("Executing lifecycle version %s", style.Symbol(lifecycleVersion))
	}

	l.logger.Verbose(style.Step("DETECTING"))
	if err := l.Detect(ctx); err != nil {
		return err
	}

	l.logger.Verbose(style.Step("RESTORING"))
	if opts.ClearCache {
		l.logger.Verbose("Skipping 'restore' due to clearing cache")
	} else if err := l.Restore(ctx, cacheImage.Image()); err != nil {
		return err
	}

	l.logger.Verbose(style.Step("ANALYZING"))
	if opts.ClearCache {
		l.logger.Verbose("Skipping 'analyze' due to clearing cache")
	} else {
		if err := l.Analyze(ctx, opts.Image.Name(), opts.Publish); err != nil {
			return err
		}
	}

	l.logger.Verbose(style.Step("BUILDING"))
	if err := l.Build(ctx); err != nil {
		return err
	}

	l.logger.Verbose(style.Step("EXPORTING"))
	if err := l.Export(ctx, opts.Image.Name(), opts.RunImage, opts.Publish); err != nil {
		return err
	}

	l.logger.Verbose(style.Step("CACHING"))
	if err := l.Cache(ctx, cacheImage.Image()); err != nil {
		return err
	}
	return nil
}

func (l *Lifecycle) Setup(appDir string, builder *builder.Builder) {
	l.LayersVolume = "pack-layers-" + randString(10)
	l.AppVolume = "pack-app-" + randString(10)
	l.appDir = appDir
	l.appOnce = &sync.Once{}
	l.Builder = builder
}

func (l *Lifecycle) Cleanup() error {
	var reterr error
	if err := l.docker.VolumeRemove(context.Background(), l.LayersVolume, true); err != nil {
		reterr = errors.Wrapf(err, "failed to clean up layers volume %s", l.LayersVolume)
	}
	if err := l.docker.VolumeRemove(context.Background(), l.AppVolume, true); err != nil {
		reterr = errors.Wrapf(err, "failed to clean up app volume %s", l.AppVolume)
	}
	return reterr
}

func randString(n int) string {
	b := make([]byte, n)
	for i := range b {
		b[i] = 'a' + byte(rand.Intn(26))
	}
	return string(b)
}
