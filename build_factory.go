package pack

import (
	"archive/tar"
	"bytes"
	"context"
	"crypto/md5"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/buildpack/pack/cache"
	"github.com/buildpack/pack/config"
	"github.com/buildpack/pack/containers"
	"github.com/buildpack/pack/docker"
	"github.com/buildpack/pack/fs"
	"github.com/buildpack/pack/logging"
	"github.com/buildpack/pack/style"

	"github.com/BurntSushi/toml"
	"github.com/buildpack/lifecycle"
	"github.com/buildpack/lifecycle/image"
	"github.com/buildpack/lifecycle/image/auth"
	dockertypes "github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/pkg/errors"
)

//go:generate mockgen -package mocks -destination mocks/cache.go github.com/buildpack/pack Cache
type Cache interface {
	Clear(context.Context) error
	Volume() string
}

type BuildFactory struct {
	Cli          Docker
	Logger       *logging.Logger
	FS           FS
	Config       *config.Config
	ImageFactory ImageFactory
	Cache        Cache
}

type BuildFlags struct {
	AppDir     string
	Builder    string
	RunImage   string
	Env        []string
	EnvFile    string
	RepoName   string
	Publish    bool
	NoPull     bool
	ClearCache bool
	Buildpacks []string
}

type BuildConfig struct {
	AppDir     string
	Builder    string
	RunImage   string
	Env        map[string]string
	RepoName   string
	Publish    bool
	NoPull     bool
	ClearCache bool
	Buildpacks []string
	// Above are copied from BuildFlags are set by init
	Cli    Docker
	Logger *logging.Logger
	FS     FS
	Config *config.Config
	// Above are copied from BuildFactory
	Cache Cache
}

const (
	launchDir     = "/workspace"
	buildpacksDir = "/buildpacks"
	platformDir   = "/platform"
	orderPath     = "/buildpacks/order.toml"
	groupPath     = `/workspace/group.toml`
	planPath      = "/workspace/plan.toml"
)

func DefaultBuildFactory(logger *logging.Logger, cache Cache, dockerClient Docker, imageFactory ImageFactory) (*BuildFactory, error) {
	f := &BuildFactory{
		ImageFactory: imageFactory,
		Logger:       logger,
		FS:           &fs.FS{},
		Cache:        cache,
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

func (bf *BuildFactory) BuildConfigFromFlags(f *BuildFlags) (*BuildConfig, error) {
	if f.AppDir == "" {
		var err error
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
		AppDir:     appDir,
		RepoName:   f.RepoName,
		Publish:    f.Publish,
		NoPull:     f.NoPull,
		ClearCache: f.ClearCache,
		Buildpacks: f.Buildpacks,
		Cli:        bf.Cli,
		Logger:     bf.Logger,
		FS:         bf.FS,
		Config:     bf.Config,
	}

	if f.EnvFile != "" {
		b.Env, err = parseEnvFile(f.EnvFile)
		if err != nil {
			return nil, err
		}
	} else {
		b.Env = map[string]string{}
	}
	for _, item := range f.Env {
		b.Env = addEnvVar(b.Env, item)
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
	}

	builderImage, err := bf.ImageFactory.NewLocal(b.Builder, !f.NoPull)
	if err != nil {
		return nil, err
	}

	builderStackID, err := builderImage.Label(StackLabel)
	if err != nil {
		return nil, fmt.Errorf("invalid builder image %s: %s", style.Symbol(b.Builder), err)
	}
	if builderStackID == "" {
		return nil, fmt.Errorf("invalid builder image %s: missing required label %s", style.Symbol(b.Builder), style.Symbol(StackLabel))
	}

	if f.RunImage != "" {
		bf.Logger.Verbose("Using user-provided run image %s", style.Symbol(f.RunImage))
		b.RunImage = f.RunImage
	} else {
		label, err := builderImage.Label(BuilderMetadataLabel)
		if err != nil {
			return nil, fmt.Errorf("invalid builder image %s: %s", style.Symbol(b.Builder), err)
		}
		if label == "" {
			return nil, fmt.Errorf("invalid builder image %s: missing required label %s -- try recreating builder", style.Symbol(b.Builder), style.Symbol(BuilderMetadataLabel))
		}
		var builderMetadata BuilderImageMetadata
		if err := json.Unmarshal([]byte(label), &builderMetadata); err != nil {
			return nil, fmt.Errorf("invalid builder image metadata: %s", err)
		}

		reg, err := config.Registry(f.RepoName)
		if err != nil {
			return nil, err
		}
		var overrideRunImages []string
		if runImage := bf.Config.GetRunImage(builderMetadata.RunImage.Image); runImage != nil {
			overrideRunImages = runImage.Mirrors
		}
		i := append(overrideRunImages, append([]string{builderMetadata.RunImage.Image}, builderMetadata.RunImage.Mirrors...)...)
		b.RunImage, err = config.ImageByRegistry(reg, i)
		if err != nil {
			return nil, err
		}
		b.Logger.Verbose("Selected run image %s from builder %s", style.Symbol(b.RunImage), style.Symbol(b.Builder))
	}

	var runImage image.Image
	if f.Publish {
		runImage, err = bf.ImageFactory.NewRemote(b.RunImage)
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
		}
		runImage, err = bf.ImageFactory.NewLocal(b.RunImage, !f.NoPull)
		if err != nil {
			return nil, err
		}

		if found, err := runImage.Found(); !found {
			return nil, fmt.Errorf("local run image %s does not exist", style.Symbol(b.RunImage))
		} else if err != nil {
			return nil, fmt.Errorf("invalid run image %s: %s", style.Symbol(b.RunImage), err)
		}
	}

	if runStackID, err := runImage.Label(StackLabel); err != nil {
		return nil, fmt.Errorf("invalid run image %s: %s", style.Symbol(b.RunImage), err)
	} else if runStackID == "" {
		return nil, fmt.Errorf("invalid run image %s: missing required label %s", style.Symbol(b.RunImage), style.Symbol(StackLabel))
	} else if builderStackID != runStackID {
		return nil, fmt.Errorf("invalid stack: stack %s from run image %s does not match stack %s from builder image %s", style.Symbol(runStackID), style.Symbol(b.RunImage), style.Symbol(builderStackID), style.Symbol(b.Builder))
	}

	b.Cache = bf.Cache
	bf.Logger.Verbose(fmt.Sprintf("Using cache volume %s", style.Symbol(b.Cache.Volume())))

	return b, nil
}

func Build(ctx context.Context, outWriter, errWriter io.Writer, appDir, buildImage, runImage, repoName string, publish, clearCache bool) error {
	// TODO: Receive Cache as an argument of this function
	dockerClient, err := docker.New()
	if err != nil {
		return err
	}
	c, err := cache.New(repoName, dockerClient)
	if err != nil {
		return err
	}

	imageFactory, err := image.NewFactory(image.WithOutWriter(outWriter))
	if err != nil {
		return err
	}
	logger := logging.NewLogger(outWriter, errWriter, true, false)
	bf, err := DefaultBuildFactory(logger, c, dockerClient, imageFactory)
	if err != nil {
		return err
	}
	b, err := bf.BuildConfigFromFlags(&BuildFlags{
		AppDir:     appDir,
		Builder:    buildImage,
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
	if err := b.Detect(ctx); err != nil {
		return err
	}

	b.Logger.Verbose(style.Step("ANALYZING"))
	b.Logger.Verbose("Reading information from previous image for possible re-use")
	if err := b.Analyze(ctx); err != nil {
		return err
	}

	b.Logger.Verbose(style.Step("BUILDING"))
	if err := b.Build(ctx); err != nil {
		return err
	}

	b.Logger.Verbose(style.Step("EXPORTING"))
	if err := b.Export(ctx); err != nil {
		return err
	}

	return nil
}

func (b *BuildConfig) parseBuildpack(ref string) (string, string) {
	parts := strings.Split(ref, "@")
	if len(parts) == 2 {
		return parts[0], parts[1]
	}
	b.Logger.Verbose("No version for %s buildpack provided, will use %s", style.Symbol(parts[0]), style.Symbol(parts[0]+"@latest"))
	return parts[0], "latest"
}

func (b *BuildConfig) copyBuildpacksToContainer(ctx context.Context, ctrID string) ([]*lifecycle.Buildpack, error) {
	var buildpacks []*lifecycle.Buildpack
	for _, bp := range b.Buildpacks {
		var id, version string
		if _, err := os.Stat(filepath.Join(bp, "buildpack.toml")); !os.IsNotExist(err) {
			if runtime.GOOS == "windows" {
				return nil, fmt.Errorf("directory buildpacks are not implemented on windows")
			}
			var buildpackTOML struct {
				Buildpack Buildpack
			}

			_, err = toml.DecodeFile(filepath.Join(bp, "buildpack.toml"), &buildpackTOML)
			if err != nil {
				return nil, fmt.Errorf(`failed to decode buildpack.toml from "%s": %s`, bp, err)
			}
			id = buildpackTOML.Buildpack.ID
			version = buildpackTOML.Buildpack.Version
			bpDir := filepath.Join(buildpacksDir, buildpackTOML.Buildpack.escapedID(), version)
			ftr, errChan := b.FS.CreateTarReader(bp, bpDir, 0, 0)
			if err := b.Cli.CopyToContainer(ctx, ctrID, "/", ftr, dockertypes.CopyToContainerOptions{}); err != nil {
				return nil, errors.Wrapf(err, "copying buildpack '%s' to container", bp)
			}
			if err := <-errChan; err != nil {
				return nil, errors.Wrapf(err, "copying buildpack '%s' to container", bp)
			}
		} else {
			id, version = b.parseBuildpack(bp)
		}
		buildpacks = append(
			buildpacks,
			&lifecycle.Buildpack{ID: id, Version: version, Optional: false},
		)
	}
	return buildpacks, nil
}

func (b *BuildConfig) Detect(ctx context.Context) error {
	if b.ClearCache {
		if err := b.Cache.Clear(ctx); err != nil {
			return errors.Wrap(err, "clearing cache")
		}
		b.Logger.Verbose("Cache volume %s cleared", style.Symbol(b.Cache.Volume()))
	}

	ctr, err := b.Cli.ContainerCreate(ctx, &container.Config{
		Image: b.Builder,
		Cmd: []string{
			"/lifecycle/detector",
			"-buildpacks", buildpacksDir,
			"-order", orderPath,
			"-group", groupPath,
			"-plan", planPath,
		},
		Labels: map[string]string{"author": "pack"},
	}, &container.HostConfig{
		Binds: []string{
			fmt.Sprintf("%s:%s:", b.Cache.Volume(), launchDir),
		},
	}, nil, "")
	if err != nil {
		return errors.Wrap(err, "create detect container")
	}
	defer containers.Remove(b.Cli, ctr.ID)

	var orderToml string
	b.Logger.Verbose(style.Step("DETECTING"))
	if len(b.Buildpacks) == 0 {
		orderToml = "" // use order.toml already in image
	} else {
		b.Logger.Verbose("Using manually-provided group")

		buildpacks, err := b.copyBuildpacksToContainer(ctx, ctr.ID)
		if err != nil {
			return errors.Wrap(err, "copy buildpacks to container")
		}

		groups := lifecycle.BuildpackOrder{
			lifecycle.BuildpackGroup{
				Buildpacks: buildpacks,
			},
		}

		var tomlBuilder strings.Builder
		if err := toml.NewEncoder(&tomlBuilder).Encode(map[string]interface{}{"groups": groups}); err != nil {
			return errors.Wrapf(err, "encoding order.toml: %#v", groups)
		}

		orderToml = tomlBuilder.String()
	}

	tr, errChan := b.FS.CreateTarReader(b.AppDir, launchDir+"/app", 0, 0)
	if err := b.Cli.CopyToContainer(ctx, ctr.ID, "/", tr, dockertypes.CopyToContainerOptions{}); err != nil {
		return errors.Wrap(err, "copy app to workspace volume")
	}

	if err := <-errChan; err != nil {
		return errors.Wrap(err, "copy app to workspace volume")
	}

	uid, gid, err := b.packUidGid(ctx, b.Builder)
	if err != nil {
		return errors.Wrap(err, "get pack uid gid")
	}
	if err := b.chownDir(ctx, launchDir+"/app", uid, gid); err != nil {
		return errors.Wrap(err, "chown app to workspace volume")
	}

	if orderToml != "" {
		ftr, err := b.FS.CreateSingleFileTar(orderPath, orderToml)
		if err != nil {
			return errors.Wrap(err, "converting order TOML to tar reader")
		}
		if err := b.Cli.CopyToContainer(ctx, ctr.ID, "/", ftr, dockertypes.CopyToContainerOptions{}); err != nil {
			return errors.Wrap(err, fmt.Sprintf("creating %s", orderPath))
		}
	}

	if err := b.copyEnvsToContainer(ctx, ctr.ID); err != nil {
		return err
	}

	if err := b.Cli.RunContainer(
		ctx,
		ctr.ID,
		b.Logger.VerboseWriter().WithPrefix("detector"),
		b.Logger.VerboseErrorWriter().WithPrefix("detector"),
	); err != nil {
		return errors.Wrap(err, "run detect container")
	}
	return nil
}

func (b *BuildConfig) Analyze(ctx context.Context) error {
	ctrConf := &container.Config{
		Image:  b.Builder,
		Labels: map[string]string{"author": "pack"},
	}
	hostConfig := &container.HostConfig{
		Binds: []string{
			fmt.Sprintf("%s:%s:", b.Cache.Volume(), launchDir),
		},
	}

	if b.Publish {
		authHeader, err := auth.BuildEnvVar(authn.DefaultKeychain, b.RepoName, b.RunImage)
		if err != nil {
			return err
		}

		ctrConf.Env = []string{fmt.Sprintf(`CNB_REGISTRY_AUTH=%s`, authHeader)}
		ctrConf.Cmd = []string{
			"/lifecycle/analyzer",
			"-layers", launchDir,
			"-group", groupPath,
			b.RepoName,
		}
		hostConfig.NetworkMode = "host"
	} else {
		ctrConf.Cmd = []string{
			"/lifecycle/analyzer",
			"-layers", launchDir,
			"-group", groupPath,
			"-daemon",
			b.RepoName,
		}
		ctrConf.User = "root"
		hostConfig.Binds = append(hostConfig.Binds, "/var/run/docker.sock:/var/run/docker.sock")
	}

	ctr, err := b.Cli.ContainerCreate(ctx, ctrConf, hostConfig, nil, "")
	if err != nil {
		return errors.Wrap(err, "create analyze container")
	}
	defer containers.Remove(b.Cli, ctr.ID)

	if err := b.Cli.RunContainer(
		ctx,
		ctr.ID,
		b.Logger.VerboseWriter().WithPrefix("analyzer"),
		b.Logger.VerboseErrorWriter().WithPrefix("analyzer"),
	); err != nil {
		return errors.Wrap(err, "run analyze container")
	}

	uid, gid, err := b.packUidGid(ctx, b.Builder)
	if err != nil {
		return errors.Wrap(err, "get pack uid and gid")
	}
	if err := b.chownDir(ctx, launchDir, uid, gid); err != nil {
		return errors.Wrap(err, "chown launch dir")
	}

	return nil
}

func (b *BuildConfig) Build(ctx context.Context) error {
	ctr, err := b.Cli.ContainerCreate(ctx, &container.Config{
		Image: b.Builder,
		Cmd: []string{
			"/lifecycle/builder",
			"-buildpacks", buildpacksDir,
			"-layers", launchDir,
			"-group", groupPath,
			"-plan", planPath,
			"-platform", platformDir,
		},
		Labels: map[string]string{"author": "pack"},
	}, &container.HostConfig{
		Binds: []string{
			fmt.Sprintf("%s:%s:", b.Cache.Volume(), launchDir),
		},
	}, nil, "")
	if err != nil {
		return errors.Wrap(err, "create build container")
	}
	defer containers.Remove(b.Cli, ctr.ID)

	if len(b.Buildpacks) > 0 {
		_, err = b.copyBuildpacksToContainer(ctx, ctr.ID)
		if err != nil {
			return errors.Wrap(err, "copy buildpacks to container")
		}
	}

	if err := b.copyEnvsToContainer(ctx, ctr.ID); err != nil {
		return err
	}

	if err = b.Cli.RunContainer(
		ctx,
		ctr.ID,
		b.Logger.VerboseWriter().WithPrefix("builder"),
		b.Logger.VerboseErrorWriter().WithPrefix("builder"),
	); err != nil {
		return errors.Wrap(err, "run build container")
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
		out = addEnvVar(out, line)
	}
	return out, nil
}

func addEnvVar(env map[string]string, item string) map[string]string {
	arr := strings.SplitN(item, "=", 2)
	if len(arr) > 1 {
		env[arr[0]] = arr[1]
	} else {
		env[arr[0]] = os.Getenv(arr[0])
	}
	return env
}

func (b *BuildConfig) tarEnvFile() (io.Reader, error) {
	now := time.Now()
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	for k, v := range b.Env {
		if err := tw.WriteHeader(&tar.Header{Name: "/platform/env/" + k, Size: int64(len(v)), Mode: 0444, ModTime: now}); err != nil {
			return nil, err
		}
		if _, err := tw.Write([]byte(v)); err != nil {
			return nil, err
		}
	}
	if err := tw.WriteHeader(&tar.Header{Typeflag: tar.TypeDir, Name: "/platform/env/", Mode: 0555, ModTime: now}); err != nil {
		return nil, err
	}
	if err := tw.Close(); err != nil {
		return nil, err
	}
	return bytes.NewReader(buf.Bytes()), nil
}

func (b *BuildConfig) copyEnvsToContainer(ctx context.Context, containerID string) error {
	if len(b.Env) > 0 {
		platformEnvTar, err := b.tarEnvFile()
		if err != nil {
			return errors.Wrap(err, "create env files")
		}
		if err := b.Cli.CopyToContainer(ctx, containerID, "/", platformEnvTar, dockertypes.CopyToContainerOptions{}); err != nil {
			return errors.Wrap(err, "create env files")
		}
	}
	return nil
}

func (b *BuildConfig) Export(ctx context.Context) error {
	ctrConf := &container.Config{
		Image:  b.Builder,
		Labels: map[string]string{"author": "pack"},
	}
	hostConfig := &container.HostConfig{
		Binds: []string{
			fmt.Sprintf("%s:%s:", b.Cache.Volume(), launchDir),
		},
	}

	if b.Publish {
		authHeader, err := auth.BuildEnvVar(authn.DefaultKeychain, b.RepoName, b.RunImage)
		if err != nil {
			return err
		}

		ctrConf.Env = []string{fmt.Sprintf(`CNB_REGISTRY_AUTH=%s`, authHeader)}
		ctrConf.Cmd = []string{
			"/lifecycle/exporter",
			"-image", b.RunImage,
			"-layers", launchDir,
			"-group", groupPath,
			b.RepoName,
		}
		hostConfig.NetworkMode = "host"
	} else {
		ctrConf.Cmd = []string{
			"/lifecycle/exporter",
			"-image", b.RunImage,
			"-layers", launchDir,
			"-group", groupPath,
			"-daemon",
			b.RepoName,
		}
		ctrConf.User = "root"
		hostConfig.Binds = append(hostConfig.Binds, "/var/run/docker.sock:/var/run/docker.sock")
	}

	ctr, err := b.Cli.ContainerCreate(ctx, ctrConf, hostConfig, nil, "")
	if err != nil {
		return errors.Wrap(err, "create export container")
	}
	defer containers.Remove(b.Cli, ctr.ID)

	uid, gid, err := b.packUidGid(ctx, b.Builder)
	if err != nil {
		return errors.Wrap(err, "get pack uid and gid")
	}
	if err := b.chownDir(ctx, launchDir, uid, gid); err != nil {
		return errors.Wrap(err, "chown launch dir")
	}

	if err := b.Cli.RunContainer(
		ctx,
		ctr.ID,
		b.Logger.VerboseWriter().WithPrefix("exporter"),
		b.Logger.VerboseErrorWriter().WithPrefix("exporter"),
	); err != nil {
		return errors.Wrap(err, "run export container")
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

func (b *BuildConfig) chownDir(ctx context.Context, path string, uid, gid int) error {
	ctr, err := b.Cli.ContainerCreate(ctx, &container.Config{
		Image:  b.Builder,
		Cmd:    []string{"chown", "-R", fmt.Sprintf("%d:%d", uid, gid), path},
		User:   "root",
		Labels: map[string]string{"author": "pack"},
	}, &container.HostConfig{
		Binds: []string{
			fmt.Sprintf("%s:%s:", b.Cache.Volume(), launchDir),
		},
	}, nil, "")
	if err != nil {
		return err
	}
	defer containers.Remove(b.Cli, ctr.ID)
	if err := b.Cli.RunContainer(ctx, ctr.ID, b.Logger.VerboseWriter(), b.Logger.VerboseErrorWriter()); err != nil {
		return err
	}
	return nil
}
