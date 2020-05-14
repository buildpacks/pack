package build

import (
	"context"
	"math/rand"
	"sync"
	"time"

	"github.com/docker/docker/client"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/pkg/errors"

	"github.com/buildpacks/pack/internal/builder"
	"github.com/buildpacks/pack/logging"
)

var (
	// SupportedPlatformAPIVersions lists the Platform API versions pack supports.
	SupportedPlatformAPIVersions = []string{"0.2", "0.3"}
)

type Builder interface {
	Name() string
	UID() int
	GID() int
	LifecycleDescriptor() builder.LifecycleDescriptor
	Stack() builder.StackMetadata
}

type Lifecycle struct {
	executor           *Executor
	builder            Builder
	lifecycleImage     string
	logger             logging.Logger
	docker             client.CommonAPIClient
	appPath            string
	appOnce            *sync.Once
	httpProxy          string
	httpsProxy         string
	noProxy            string
	version            string
	platformAPIVersion string
	LayersVolume       string
	AppVolume          string
	Volumes            []string
	DefaultProcessType string
	fileFilter         func(string) bool
}

type Cache interface {
	Name() string
	Clear(context.Context) error
}

func init() {
	rand.Seed(time.Now().UTC().UnixNano())
}

func NewLifecycle(docker client.CommonAPIClient, logger logging.Logger) *Lifecycle {
	l := &Lifecycle{logger: logger, docker: docker, executor: &Executor{}}

	return l
}

type LifecycleOptions struct {
	AppPath            string
	Image              name.Reference
	Builder            Builder
	LifecycleImage     string
	RunImage           string
	ClearCache         bool
	Publish            bool
	TrustBuilder       bool
	HTTPProxy          string
	HTTPSProxy         string
	NoProxy            string
	Network            string
	Volumes            []string
	DefaultProcessType string
	FileFilter         func(string) bool
}

func (l *Lifecycle) Execute(ctx context.Context, opts LifecycleOptions) error {
	l.Setup(opts)
	defer l.Cleanup()

	phaseFactory := NewDefaultPhaseFactory(l)

	return l.executor.Execute(ctx, opts, l, l.platformAPIVersion, phaseFactory, l.docker, l.logger)
}

func (l *Lifecycle) Setup(opts LifecycleOptions) {
	l.LayersVolume = "pack-layers-" + randString(10)
	l.AppVolume = "pack-app-" + randString(10)
	l.appPath = opts.AppPath
	l.appOnce = &sync.Once{}
	l.builder = opts.Builder
	l.lifecycleImage = opts.LifecycleImage
	l.httpProxy = opts.HTTPProxy
	l.httpsProxy = opts.HTTPSProxy
	l.noProxy = opts.NoProxy
	l.version = opts.Builder.LifecycleDescriptor().Info.Version.String()
	l.platformAPIVersion = opts.Builder.LifecycleDescriptor().API.PlatformVersion.String()
	l.DefaultProcessType = opts.DefaultProcessType
	l.fileFilter = opts.FileFilter
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
