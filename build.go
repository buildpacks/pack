package pack

import (
	"archive/tar"
	"bytes"
	"context"
	"crypto/md5"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/buildpack/lifecycle/image"
	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"

	"github.com/buildpack/pack/config"
	"github.com/buildpack/pack/docker"
	"github.com/buildpack/pack/fs"

	"github.com/BurntSushi/toml"
	"github.com/buildpack/lifecycle"
	dockertypes "github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/pkg/errors"
)

type BuildFactory struct {
	Cli          Docker
	Stdout       io.Writer
	Stderr       io.Writer
	Log          *log.Logger
	FS           FS
	Config       *config.Config
	ImageFactory ImageFactory
}

type BuildFlags struct {
	AppDir     string
	Builder    string
	RunImage   string
	EnvFile    string
	RepoName   string
	Publish    bool
	NoPull     bool
	Buildpacks []string
}

type BuildConfig struct {
	AppDir     string
	Builder    string
	RunImage   string
	EnvFile    map[string]string
	RepoName   string
	Publish    bool
	NoPull     bool
	Buildpacks []string
	// Above are copied from BuildFlags are set by init
	Cli    Docker
	Stdout io.Writer
	Stderr io.Writer
	Log    *log.Logger
	FS     FS
	Config *config.Config
	// Above are copied from BuildFactory
	CacheVolume string
}

const (
	launchDir     = "/workspace"
	cacheDir      = "/cache"
	buildpacksDir = "/buildpacks"
	platformDir   = "/platform"
	orderPath     = "/buildpacks/order.toml"
	groupPath     = `/workspace/group.toml`
	planPath      = "/workspace/plan.toml"
)

func DefaultBuildFactory() (*BuildFactory, error) {
	f := &BuildFactory{
		Stdout: os.Stdout,
		Stderr: os.Stderr,
		Log:    log.New(os.Stdout, "", log.LstdFlags),
		FS:     &fs.FS{},
	}

	var err error
	f.Cli, err = docker.New()
	if err != nil {
		return nil, err
	}

	f.Config, err = config.NewDefault()
	if err != nil {
		return nil, err
	}

	f.ImageFactory, err = image.DefaultFactory()
	if err != nil {
		return nil, err
	}

	return f, nil
}

func (bf *BuildFactory) BuildConfigFromFlags(f *BuildFlags) (*BuildConfig, error) {
	if f.AppDir == "current working directory" { // default placeholder
		var err error
		f.AppDir, err = os.Getwd()
		if err != nil {
			return nil, err
		}
		bf.Log.Printf("Defaulting app directory to current working directory '%s' (use --path to override)", f.AppDir)
	}
	appDir, err := filepath.Abs(f.AppDir)
	if err != nil {
		return nil, err
	}

	if f.RepoName == "" {
		f.RepoName = fmt.Sprintf("pack.local/run/%x", md5.Sum([]byte(appDir)))
	}

	b := &BuildConfig{
		AppDir:     appDir,
		RepoName:   f.RepoName,
		Publish:    f.Publish,
		NoPull:     f.NoPull,
		Buildpacks: f.Buildpacks,
		Cli:        bf.Cli,
		Stdout:     bf.Stdout,
		Stderr:     bf.Stderr,
		Log:        bf.Log,
		FS:         bf.FS,
		Config:     bf.Config,
		CacheVolume: fmt.Sprintf("pack-cache-%x", md5.Sum([]byte(appDir))),
	}

	if f.EnvFile != "" {
		b.EnvFile, err = parseEnvFile(f.EnvFile)
		if err != nil {
			return nil, err
		}
	}

	if f.Builder == "" {
		bf.Log.Printf("Using default builder image '%s'\n", bf.Config.DefaultBuilder)
		b.Builder = bf.Config.DefaultBuilder
	} else {
		bf.Log.Printf("Using user provided builder image '%s'\n", f.Builder)
		b.Builder = f.Builder
	}
	if !f.NoPull {
		bf.Log.Printf("Pulling builder image '%s' (use --no-pull flag to skip this step)", b.Builder)
	}

	builderImage, err := bf.ImageFactory.NewLocal(b.Builder, !f.NoPull)
	if err != nil {
		return nil, err
	}

	builderStackID, err := builderImage.Label("io.buildpacks.stack.id")
	if err != nil {
		return nil, fmt.Errorf(`invalid builder image "%s": %s`, b.Builder, err)
	}
	if builderStackID == "" {
		return nil, fmt.Errorf(`invalid builder image "%s": missing required label "io.buildpacks.stack.id"`, b.Builder)
	}
	stack, err := bf.Config.Get(builderStackID)
	if err != nil {
		return nil, err
	}

	if f.RunImage != "" {
		bf.Log.Printf("Using user provided run image '%s'\n", f.RunImage)
		b.RunImage = f.RunImage
	} else {
		reg, err := config.Registry(f.RepoName)
		if err != nil {
			return nil, err
		}
		b.RunImage, err = config.ImageByRegistry(reg, stack.RunImages)
		if err != nil {
			return nil, err
		}
		b.Log.Printf("Selected run image '%s' from stack '%s'\n", b.RunImage, builderStackID)
	}

	var runImage image.Image
	if f.Publish {
		runImage, err = bf.ImageFactory.NewRemote(b.RunImage)
		if err != nil {
			return nil, err
		}
	} else {
		if !f.NoPull {
			bf.Log.Printf("Pulling run image '%s' (use --no-pull flag to skip this step)", b.RunImage)
		}
		runImage, err = bf.ImageFactory.NewLocal(b.RunImage, !f.NoPull)
		if err != nil {
			return nil, err
		}
	}

	if runStackID, err := runImage.Label("io.buildpacks.stack.id"); err != nil {
		return nil, fmt.Errorf(`invalid run image "%s": %s`, b.RunImage, err)
	} else if runStackID == "" {
		return nil, fmt.Errorf(`invalid run image "%s": missing required label "io.buildpacks.stack.id"`, b.RunImage)
	} else if builderStackID != runStackID {
		return nil, fmt.Errorf(`invalid stack: stack "%s" from run image "%s" does not match stack "%s" from builder image "%s"`, runStackID, b.RunImage, builderStackID, b.Builder)
	}

	return b, nil
}

func Build(appDir, buildImage, runImage, repoName string, publish bool) error {
	bf, err := DefaultBuildFactory()
	if err != nil {
		return err
	}
	b, err := bf.BuildConfigFromFlags(&BuildFlags{
		AppDir:   appDir,
		Builder:  buildImage,
		RunImage: runImage,
		RepoName: repoName,
		Publish:  publish,
	})
	if err != nil {
		return err
	}
	return b.Run()
}

func (b *BuildConfig) Run() error {
	if err := b.Detect(); err != nil {
		return err
	}

	fmt.Println("*** ANALYZING: Reading information from previous image for possible re-use")
	if err := b.Analyze(); err != nil {
		return err
	}

	fmt.Println("*** BUILDING:")
	if err := b.Build(); err != nil {
		return err
	}

	fmt.Println("*** EXPORTING:")
	if err := b.Export(); err != nil {
		return err
	}

	return nil
}

func parseBuildpack(ref string) (string, string) {
	parts := strings.Split(ref, "@")
	if len(parts) == 2 {
		return parts[0], parts[1]
	}
	fmt.Printf("No version for '%s' buildpack provided, will use '%s@latest'\n", parts[0], parts[0])
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
			id, version = parseBuildpack(bp)
		}
		buildpacks = append(
			buildpacks,
			&lifecycle.Buildpack{ID: id, Version: version, Optional: false},
		)
	}
	return buildpacks, nil
}

func (b *BuildConfig) Detect() error {
	ctx := context.Background()
	ctr, err := b.Cli.ContainerCreate(ctx, &container.Config{
		Image: b.Builder,
		Cmd: []string{
			"/lifecycle/detector",
			"-buildpacks", buildpacksDir,
			"-order", orderPath,
			"-group", groupPath,
			"-plan", planPath,
		},
	}, &container.HostConfig{
		Binds: []string{
			fmt.Sprintf("%s:%s:", b.CacheVolume, launchDir),
		},
	}, nil, "")
	if err != nil {
		return errors.Wrap(err, "container create")
	}
	defer b.Cli.ContainerRemove(ctx, ctr.ID, dockertypes.ContainerRemoveOptions{})

	var orderToml string
	if len(b.Buildpacks) == 0 {
		fmt.Fprintln(b.Stdout, "*** DETECTING:")
		orderToml = "" // use order toml already in image
	} else {
		fmt.Fprintln(b.Stdout, "*** DETECTING WITH MANUALLY-PROVIDED GROUP:")

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
	if err := b.Cli.CopyToContainer(ctx, ctr.ID, "/", tr, dockertypes.CopyToContainerOptions{
	}); err != nil {
		return errors.Wrap(err, "copy app to workspace volume")
	}
	if err := <-errChan; err != nil {
		return errors.Wrap(err, "copy app to workspace volume")
	}

	uid, gid, err := b.packUidGid(b.Builder)
	if err != nil {
		return errors.Wrap(err, "get pack uid gid")
	}
	if err := b.chownDir(launchDir+"/app", uid, gid); err != nil {
		return errors.Wrap(err, "chown app to workspace volume")
	}

	if orderToml != "" {
		ftr, err := b.FS.CreateSingleFileTar(orderPath, orderToml)
		if err != nil {
			return errors.Wrap(err, "converting order TOML to tar reader")
		}
		if err := b.Cli.CopyToContainer(ctx, ctr.ID, "/", ftr, dockertypes.CopyToContainerOptions{
		}); err != nil {
			return errors.Wrap(err, fmt.Sprintf("creating %s", orderPath))
		}
	}

	if err := b.Cli.RunContainer(ctx, ctr.ID, b.Stdout, b.Stderr); err != nil {
		return errors.Wrap(err, "run detect container")
	}
	return nil
}

func (b *BuildConfig) Analyze() error {
	ctx := context.Background()
	ctrConf := &container.Config{
		Image: b.Builder,
	}
	hostConfig := &container.HostConfig{
		Binds: []string{
			fmt.Sprintf("%s:%s:", b.CacheVolume, launchDir),
		},
	}

	if b.Publish {
		authHeader, err := authHeader(b.RepoName)
		if err != nil {
			return err
		}

		ctrConf.Env = []string{fmt.Sprintf(`PACK_REGISTRY_AUTH=%s`, authHeader)}
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
		return errors.Wrap(err, "analyze container create")
	}
	defer b.Cli.ContainerRemove(ctx, ctr.ID, dockertypes.ContainerRemoveOptions{})

	if err := b.Cli.RunContainer(ctx, ctr.ID, b.Stdout, b.Stderr); err != nil {
		return errors.Wrap(err, "analyze run container")
	}

	uid, gid, err := b.packUidGid(b.Builder)
	if err != nil {
		return errors.Wrap(err, "get pack uid and gid")
	}
	if err := b.chownDir(launchDir, uid, gid); err != nil {
		return errors.Wrap(err, "chown launch dir")
	}

	return nil
}

func authHeader(repoName string) (string, error) {
	r, err := name.ParseReference(repoName, name.WeakValidation)
	if err != nil {
		return "", err
	}
	auth, err := authn.DefaultKeychain.Resolve(r.Context().Registry)
	if err != nil {
		return "", err
	}
	return auth.Authorization()
}

func (b *BuildConfig) Build() error {
	ctx := context.Background()
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
	}, &container.HostConfig{
		Binds: []string{
			fmt.Sprintf("%s:%s:", b.CacheVolume, launchDir),
		},
	}, nil, "")
	if err != nil {
		return errors.Wrap(err, "build container create")
	}
	defer b.Cli.ContainerRemove(ctx, ctr.ID, dockertypes.ContainerRemoveOptions{})

	if len(b.Buildpacks) > 0 {
		_, err = b.copyBuildpacksToContainer(ctx, ctr.ID)
		if err != nil {
			return errors.Wrap(err, "copy buildpacks to container")
		}
	}

	if len(b.EnvFile) > 0 {
		platformEnvTar, err := b.tarEnvFile()
		if err != nil {
			return errors.Wrap(err, "create env files")
		}
		if err := b.Cli.CopyToContainer(ctx, ctr.ID, "/", platformEnvTar, dockertypes.CopyToContainerOptions{}); err != nil {
			return errors.Wrap(err, "create env files")
		}
	}

	err = b.Cli.RunContainer(ctx, ctr.ID, b.Stdout, b.Stderr)
	if err != nil {
		return errors.Wrap(err, "running builder in container")
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

func (b *BuildConfig) tarEnvFile() (io.Reader, error) {
	now := time.Now()
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	for k, v := range b.EnvFile {
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

func (b *BuildConfig) Export() error {
	ctx := context.Background()
	ctrConf := &container.Config{
		Image: b.Builder,
	}
	hostConfig := &container.HostConfig{
		Binds: []string{
			fmt.Sprintf("%s:%s:", b.CacheVolume, launchDir),
		},
	}

	if b.Publish {
		authHeader, err := authHeader(b.RepoName)
		if err != nil {
			return err
		}

		ctrConf.Env = []string{fmt.Sprintf(`PACK_REGISTRY_AUTH=%s`, authHeader)}
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
	defer b.Cli.ContainerRemove(ctx, ctr.ID, dockertypes.ContainerRemoveOptions{})

	uid, gid, err := b.packUidGid(b.Builder)
	if err != nil {
		return errors.Wrap(err, "get pack uid and gid")
	}
	if err := b.chownDir(launchDir, uid, gid); err != nil {
		return errors.Wrap(err, "chown launch dir")
	}

	if err := b.Cli.RunContainer(ctx, ctr.ID, b.Stdout, b.Stderr); err != nil {
		return errors.Wrap(err, "run lifecycle/exporter")
	}
	return nil
}

func (b *BuildConfig) packUidGid(builder string) (int, int, error) {
	i, _, err := b.Cli.ImageInspectWithRaw(context.Background(), builder)
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

func (b *BuildConfig) chownDir(path string, uid, gid int) error {
	ctx := context.Background()
	ctr, err := b.Cli.ContainerCreate(ctx, &container.Config{
		Image: b.Builder,
		Cmd:   []string{"chown", "-R", fmt.Sprintf("%d:%d", uid, gid), path},
		User:  "root",
	}, &container.HostConfig{
		Binds: []string{
			fmt.Sprintf("%s:%s:", b.CacheVolume, launchDir),
		},
	}, nil, "")
	if err != nil {
		return err
	}
	defer b.Cli.ContainerRemove(ctx, ctr.ID, dockertypes.ContainerRemoveOptions{})
	if err := b.Cli.RunContainer(ctx, ctr.ID, b.Stdout, b.Stderr); err != nil {
		return err
	}
	return nil
}
