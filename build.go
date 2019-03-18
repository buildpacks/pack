package pack

import (
	"context"
	"crypto/md5"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/buildpack/pack/build"
	"github.com/buildpack/pack/builder"
	"github.com/buildpack/pack/cache"
	"github.com/buildpack/pack/config"
	"github.com/buildpack/pack/docker"
	"github.com/buildpack/pack/fs"
	"github.com/buildpack/pack/logging"
	"github.com/buildpack/pack/style"

	lcimg "github.com/buildpack/lifecycle/image"
	dockertypes "github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/pkg/errors"
)

//go:generate mockgen -package mocks -destination mocks/cache.go github.com/buildpack/pack Cache
type Cache interface {
	Clear(context.Context) error
	Image() string
}

type BuildFactory struct {
	Cli     Docker
	Logger  *logging.Logger
	FS      *fs.FS
	Config  *config.Config
	Cache   Cache
	Fetcher Fetcher
}

type BuildFlags struct {
	AppDir     string
	Builder    string
	RunImage   string
	EnvFile    string
	RepoName   string
	Publish    bool
	NoPull     bool
	ClearCache bool
	Buildpacks []string
}

type BuildConfig struct {
	Builder                   string
	RunImage                  string
	RepoName                  string
	Publish                   bool
	ClearCache                bool
	LocallyConfiguredRunImage bool
	// Above are copied from BuildFlags are set by init
	Cli    Docker
	Logger *logging.Logger
	FS     *fs.FS
	Config *config.Config
	// Above are copied from BuildFactory
	Cache           Cache
	LifecycleConfig build.LifecycleConfig
}

const (
	launchDir     = "/workspace"
	buildpacksDir = "/buildpacks"
	platformDir   = "/platform"
	orderPath     = "/buildpacks/order.toml"
	groupPath     = `/workspace/group.toml`
	planPath      = "/workspace/plan.toml"
)

func DefaultBuildFactory(logger *logging.Logger, cache Cache, dockerClient Docker, fetcher Fetcher) (*BuildFactory, error) {
	f := &BuildFactory{
		Logger:  logger,
		FS:      &fs.FS{},
		Cache:   cache,
		Fetcher: fetcher,
	}

	var err error
	f.Cli = dockerClient

	f.Config, err = config.NewDefault()
	if err != nil {
		return nil, err
	}

	return f, nil
}

func RepositoryName(logger *logging.Logger, buildFlags *BuildFlags) (string, error) {
	if buildFlags.AppDir == "" {
		var err error
		buildFlags.AppDir, err = os.Getwd()
		if err != nil {
			return "", err
		}
		logger.Verbose("Defaulting app directory to current working directory %s (use --path to override)", style.Symbol(buildFlags.AppDir))
	}

	appDir, err := filepath.Abs(buildFlags.AppDir)
	if err != nil {
		return "", err
	}
	return calculateRepositoryName(appDir, buildFlags), nil
}

func calculateRepositoryName(appDir string, buildFlags *BuildFlags) string {
	if buildFlags.RepoName == "" {
		return fmt.Sprintf("pack.local/run/%x", md5.Sum([]byte(appDir)))
	}
	return buildFlags.RepoName
}

func (bf *BuildFactory) BuildConfigFromFlags(ctx context.Context, f *BuildFlags) (*BuildConfig, error) {
	var builderImage lcimg.Image
	var err error

	if f.AppDir == "" {
		f.AppDir, err = os.Getwd()
		if err != nil {
			return nil, err
		}
		bf.Logger.Verbose("Defaulting app directory to current working directory %s (use --path to override)", style.Symbol(f.AppDir))
	}
	appDir, err := filepath.Abs(f.AppDir)
	if err != nil {
		return nil, err
	}

	f.RepoName = calculateRepositoryName(appDir, f)

	b := &BuildConfig{
		RepoName:   f.RepoName,
		Publish:    f.Publish,
		ClearCache: f.ClearCache,
		Cli:        bf.Cli,
		Logger:     bf.Logger,
		FS:         bf.FS,
		Config:     bf.Config,
	}

	var envFile map[string]string
	if f.EnvFile != "" {
		envFile, err = parseEnvFile(f.EnvFile)
		if err != nil {
			return nil, err
		}
	}

	if f.Builder == "" {
		bf.Logger.Verbose("Using default builder image %s", style.Symbol(bf.Config.DefaultBuilder))
		b.Builder = bf.Config.DefaultBuilder
	} else {
		bf.Logger.Verbose("Using user-provided builder image %s", style.Symbol(f.Builder))
		b.Builder = f.Builder
	}
	if !f.NoPull {
		bf.Logger.Verbose("Pulling builder image %s (use --no-pull flag to skip this step)", style.Symbol(b.Builder))
		builderImage, err = bf.Fetcher.FetchUpdatedLocalImage(ctx, b.Builder, bf.Logger.RawVerboseWriter())
		if err != nil {
			return nil, err
		}
	} else {
		builderImage, err = bf.Fetcher.FetchLocalImage(b.Builder)
		if err != nil {
			return nil, err
		}
	}

	if f.RunImage != "" {
		bf.Logger.Verbose("Using user-provided run image %s", style.Symbol(f.RunImage))
		b.RunImage = f.RunImage
		b.LocallyConfiguredRunImage = true
	} else {
		label, err := builderImage.Label(builder.MetadataLabel)
		if err != nil {
			return nil, fmt.Errorf("invalid builder image %s: %s", style.Symbol(b.Builder), err)
		}
		if label == "" {
			return nil, fmt.Errorf("invalid builder image %s: missing required label %s -- try recreating builder", style.Symbol(b.Builder), style.Symbol(builder.MetadataLabel))
		}
		var builderMetadata builder.Metadata
		if err := json.Unmarshal([]byte(label), &builderMetadata); err != nil {
			return nil, fmt.Errorf("invalid builder image metadata: %s", err)
		}

		var overrideRunImages []string
		if runImage := bf.Config.GetRunImage(builderMetadata.RunImage.Image); runImage != nil {
			overrideRunImages = runImage.Mirrors
		}

		b.RunImage, b.LocallyConfiguredRunImage, err = builderMetadata.RunImageForRepoName(f.RepoName, overrideRunImages)
		if err != nil {
			return nil, err
		}

		b.Logger.Verbose("Selected run image %s from builder %s", style.Symbol(b.RunImage), style.Symbol(b.Builder))
	}

	var runImage lcimg.Image
	if f.Publish {
		runImage, err = bf.Fetcher.FetchRemoteImage(b.RunImage)
		if err != nil {
			return nil, err
		}

		if found, err := runImage.Found(); !found {
			return nil, fmt.Errorf("remote run image %s does not exist", style.Symbol(b.RunImage))
		} else if err != nil {
			return nil, fmt.Errorf("invalid run image %s: %s", style.Symbol(b.RunImage), err)
		}
	} else {
		if !f.NoPull {
			bf.Logger.Verbose("Pulling run image %s (use --no-pull flag to skip this step)", style.Symbol(b.RunImage))
			runImage, err = bf.Fetcher.FetchUpdatedLocalImage(ctx, b.RunImage, b.Logger.RawVerboseWriter())
			if err != nil {
				return nil, err
			}
		} else {
			runImage, err = bf.Fetcher.FetchLocalImage(b.RunImage)
			if err != nil {
				return nil, err
			}
		}

		if found, err := runImage.Found(); !found {
			return nil, fmt.Errorf("local run image %s does not exist", style.Symbol(b.RunImage))
		} else if err != nil {
			return nil, fmt.Errorf("invalid run image %s: %s", style.Symbol(b.RunImage), err)
		}
	}

	b.Cache = bf.Cache
	bf.Logger.Verbose(fmt.Sprintf("Using cache image %s", style.Symbol(b.Cache.Image())))

	b.LifecycleConfig = build.LifecycleConfig{
		BuilderImage: b.Builder,
		Logger:       b.Logger,
		Buildpacks:   f.Buildpacks,
		EnvFile:      envFile,
		AppDir:       appDir,
	}

	return b, nil
}

func Build(ctx context.Context, outWriter, errWriter io.Writer, appDir, builderImage, runImage, repoName string, publish, clearCache bool) error {
	// TODO: Receive Cache as an argument of this function
	dockerClient, err := docker.New()
	if err != nil {
		return err
	}
	c, err := cache.New(repoName, dockerClient)
	if err != nil {
		return err
	}
	imageFactory, err := lcimg.NewFactory(lcimg.WithOutWriter(outWriter))
	if err != nil {
		return err
	}
	imageFetcher := &ImageFetcher{
		Factory: imageFactory,
		Docker:  dockerClient,
	}
	logger := logging.NewLogger(outWriter, errWriter, true, false)
	bf, err := DefaultBuildFactory(logger, c, dockerClient, imageFetcher)
	if err != nil {
		return err
	}
	b, err := bf.BuildConfigFromFlags(ctx,
		&BuildFlags{
			AppDir:     appDir,
			Builder:    builderImage,
			RunImage:   runImage,
			RepoName:   repoName,
			Publish:    publish,
			ClearCache: clearCache,
		})
	if err != nil {
		return err
	}
	return b.Run(ctx)
}

func (b *BuildConfig) Run(ctx context.Context) error {
	if b.ClearCache {
		if err := b.Cache.Clear(ctx); err != nil {
			return errors.Wrap(err, "clearing cache")
		}
		b.Logger.Verbose("Cache image %s cleared", style.Symbol(b.Cache.Image()))
	}
	lifecycle, err := build.NewLifecycle(b.LifecycleConfig)
	if err != nil {
		return err
	}
	defer lifecycle.Cleanup()

	b.Logger.Verbose(style.Step("DETECTING"))
	if err := b.detect(ctx, lifecycle); err != nil {
		return err
	}

	b.Logger.Verbose(style.Step("RESTORING"))
	if err := b.restore(ctx, lifecycle); err != nil {
		return err
	}

	b.Logger.Verbose(style.Step("ANALYZING"))
	b.Logger.Verbose("Reading information from previous image for possible re-use")
	if err := b.analyze(ctx, lifecycle); err != nil {
		return err
	}

	b.Logger.Verbose(style.Step("BUILDING"))
	if err := b.build(ctx, lifecycle); err != nil {
		return err
	}

	b.Logger.Verbose(style.Step("EXPORTING"))
	if err := b.export(ctx, lifecycle); err != nil {
		return err
	}

	b.Logger.Verbose(style.Step("CACHING"))
	if err := b.cache(ctx, lifecycle); err != nil {
		return err
	}

	return nil
}

func (b *BuildConfig) detect(ctx context.Context, lifecycle *build.Lifecycle) error {
	phase, err := lifecycle.NewPhase(
		"detector",
		build.WithArgs("-buildpacks", buildpacksDir,
			"-order", orderPath,
			"-group", groupPath,
			"-plan", planPath,
		),
	)
	if err != nil {
		return err
	}
	defer phase.Cleanup()

	if err := phase.Run(ctx); err != nil {
		return errors.Wrap(err, "run detect container")
	}
	return nil
}

func (b *BuildConfig) restore(ctx context.Context, lifecycle *build.Lifecycle) error {
	phase, err := lifecycle.NewPhase(
		"restorer",
		build.WithArgs("-image="+b.Cache.Image()),
		build.WithDaemonAccess(),
	)

	if err != nil {
		return err
	}
	defer phase.Cleanup()

	if err := phase.Run(ctx); err != nil {
		return errors.Wrap(err, "run restorer container")
	}

	return nil
}

func (b *BuildConfig) analyze(ctx context.Context, lifecycle *build.Lifecycle) error {
	var analyze *build.Phase
	var err error
	if b.Publish {
		analyze, err = lifecycle.NewPhase(
			"analyzer",
			build.WithRegistryAccess(b.RepoName, b.RunImage),
			build.WithArgs("-layers", launchDir, "-group", groupPath, b.RepoName),
		)
	} else {
		analyze, err = lifecycle.NewPhase(
			"analyzer",
			build.WithDaemonAccess(),
			build.WithArgs("-layers", launchDir, "-group", groupPath, "-daemon", b.RepoName),
		)
	}
	defer analyze.Cleanup()
	if err = analyze.Run(ctx); err != nil {
		return err
	}

	uid, gid, err := b.packUidGid(ctx, b.Builder)
	if err != nil {
		return errors.Wrap(err, "get pack uid and gid")
	}
	if err := b.chownDir(ctx, lifecycle, launchDir, uid, gid); err != nil {
		return errors.Wrap(err, "chown launch dir")
	}

	return nil
}

func (b *BuildConfig) build(ctx context.Context, lifecycle *build.Lifecycle) error {
	build, err := lifecycle.NewPhase(
		"builder",
		build.WithArgs(
			"-buildpacks", buildpacksDir,
			"-layers", launchDir,
			"-group", groupPath,
			"-plan", planPath,
			"-platform", platformDir,
		),
	)
	if err != nil {
		return err
	}
	defer build.Cleanup()
	if err := build.Run(ctx); err != nil {
		return errors.Wrap(err, "run build container")
	}
	return nil
}

type exporterArgs struct {
	args     []string
	repoName string
}

func (e *exporterArgs) label(s string) {
	e.args = append(e.args, "-label", s)
}

func (e *exporterArgs) add(args ...string) {
	e.args = append(e.args, args...)
}

func (e *exporterArgs) daemon() {
	e.args = append(e.args, "-daemon")
}

func (e *exporterArgs) list() []string {
	e.args = append(e.args, e.repoName)
	return e.args
}

func (b *BuildConfig) export(ctx context.Context, lifecycle *build.Lifecycle) error {
	var export *build.Phase
	var err error

	args := &exporterArgs{repoName: b.RepoName}

	args.add("-image", b.RunImage,
		"-layers", launchDir,
		"-group", groupPath)

	if !b.LocallyConfiguredRunImage {
		args.label("io.buildpacks.run-image=" + b.RunImage)
	}

	if b.Publish {
		export, err = lifecycle.NewPhase(
			"exporter",
			build.WithRegistryAccess(b.RepoName, b.RunImage),
			build.WithArgs(args.list()...),
		)
	} else {
		args.daemon()

		export, err = lifecycle.NewPhase(
			"exporter",
			build.WithDaemonAccess(),
			build.WithArgs(args.list()...),
		)
	}
	defer export.Cleanup()
	uid, gid, err := b.packUidGid(ctx, b.Builder)
	if err != nil {
		return errors.Wrap(err, "get pack uid and gid")
	}
	if err := b.chownDir(ctx, lifecycle, launchDir, uid, gid); err != nil {
		return errors.Wrap(err, "chown launch dir")
	}
	if err = export.Run(ctx); err != nil {
		return err
	}
	return nil
}

func (b *BuildConfig) cache(ctx context.Context, lifecycle *build.Lifecycle) error {
	phase, err := lifecycle.NewPhase(
		"cacher",
		build.WithArgs("-image="+b.Cache.Image()),
		build.WithDaemonAccess(),
	)

	if err != nil {
		return err
	}
	defer phase.Cleanup()

	if err := phase.Run(ctx); err != nil {
		return errors.Wrap(err, "run cacher container")
	}

	return nil
}

func (b *BuildConfig) packUidGid(ctx context.Context, builder string) (int, int, error) {
	i, _, err := b.Cli.ImageInspectWithRaw(ctx, builder)
	if err != nil {
		return 0, 0, errors.Wrap(err, "reading builder env variables")
	}
	var sUID, sGID string
	for _, kv := range i.Config.Env {
		kv2 := strings.SplitN(kv, "=", 2)
		if len(kv2) == 2 && kv2[0] == "PACK_USER_ID" {
			sUID = kv2[1]
		} else if len(kv2) == 2 && kv2[0] == "PACK_GROUP_ID" {
			sGID = kv2[1]
		}
	}
	if sUID == "" || sGID == "" {
		return 0, 0, errors.New("not found pack uid & gid")
	}
	var uid, gid int
	uid, err = strconv.Atoi(sUID)
	if err != nil {
		return 0, 0, errors.Wrapf(err, "parsing pack uid: %s", sUID)
	}
	gid, err = strconv.Atoi(sGID)
	if err != nil {
		return 0, 0, errors.Wrapf(err, "parsing pack gid: %s", sGID)
	}
	return uid, gid, nil
}

func (b *BuildConfig) chownDir(ctx context.Context, lifecycle *build.Lifecycle, path string, uid, gid int) error {
	ctr, err := b.Cli.ContainerCreate(ctx, &container.Config{
		Image:  b.Builder,
		Cmd:    []string{"chown", "-R", fmt.Sprintf("%d:%d", uid, gid), path},
		User:   "root",
		Labels: map[string]string{"author": "pack"},
	}, &container.HostConfig{
		Binds: []string{
			fmt.Sprintf("%s:%s:", lifecycle.WorkspaceVolume, launchDir),
		},
	}, nil, "")
	if err != nil {
		return err
	}
	defer b.Cli.ContainerRemove(context.Background(), ctr.ID, dockertypes.ContainerRemoveOptions{Force: true})
	if err := b.Cli.RunContainer(ctx, ctr.ID, b.Logger.VerboseWriter(), b.Logger.VerboseErrorWriter()); err != nil {
		return err
	}
	return nil
}

func parseEnvFile(envFile string) (map[string]string, error) {
	out := make(map[string]string, 0)
	f, err := ioutil.ReadFile(envFile)
	if err != nil {
		return nil, errors.Wrapf(err, "open %s", envFile)
	}
	for _, line := range strings.Split(string(f), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		arr := strings.SplitN(line, "=", 2)
		if len(arr) > 1 {
			out[arr[0]] = arr[1]
		} else {
			out[arr[0]] = os.Getenv(arr[0])
		}
	}
	return out, nil
}
