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
	"log"
	"math/rand"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/BurntSushi/toml"
	"github.com/buildpack/lifecycle"
	"github.com/buildpack/lifecycle/img"
	"github.com/buildpack/pack/config"
	"github.com/buildpack/pack/docker"
	"github.com/buildpack/pack/fs"
	"github.com/buildpack/pack/image"
	dockertypes "github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	dockercli "github.com/docker/docker/client"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/google/uuid"
	"github.com/pkg/errors"
)

type BuildFactory struct {
	Cli    Docker
	Stdout io.Writer
	Stderr io.Writer
	Log    *log.Logger
	FS     FS
	Config *config.Config
	Images Images
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
	Images Images
	// Above are copied from BuildFactory
	WorkspaceVolume string
	CacheVolume     string
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
		Images: &image.Client{},
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
		AppDir:          appDir,
		RepoName:        f.RepoName,
		Publish:         f.Publish,
		NoPull:          f.NoPull,
		Buildpacks:      f.Buildpacks,
		Cli:             bf.Cli,
		Stdout:          bf.Stdout,
		Stderr:          bf.Stderr,
		Log:             bf.Log,
		FS:              bf.FS,
		Config:          bf.Config,
		Images:          bf.Images,
		WorkspaceVolume: fmt.Sprintf("pack-workspace-%x", uuid.New().String()),
		CacheVolume:     fmt.Sprintf("pack-cache-%x", md5.Sum([]byte(appDir))),
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
		if err := bf.Cli.PullImage(b.Builder); err != nil {
			return nil, err
		}
	}

	builderStackID, err := b.imageLabel(b.Builder, "io.buildpacks.stack.id", true)
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

	if !f.NoPull && !f.Publish {
		bf.Log.Printf("Pulling run image '%s' (use --no-pull flag to skip this step)", b.RunImage)
		if err := bf.Cli.PullImage(b.RunImage); err != nil {
			return nil, err
		}
	}

	if runStackID, err := b.imageLabel(b.RunImage, "io.buildpacks.stack.id", !f.Publish); err != nil {
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
	defer b.Cli.VolumeRemove(context.Background(), b.WorkspaceVolume, true)

	group, err := b.Detect()
	if err != nil {
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
	if err := b.Export(group); err != nil {
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
			var buildpackTOML Buildpack
			_, err = toml.DecodeFile(filepath.Join(bp, "buildpack.toml"), &buildpackTOML)
			if err != nil {
				return nil, fmt.Errorf(`failed to decode buildpack.toml from "%s": %s`, bp, err)
			}
			id = buildpackTOML.ID
			version = buildpackTOML.Version
			bpDir := filepath.Join(buildpacksDir, buildpackTOML.escapedID(), version)
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

func (b *BuildConfig) Detect() (*lifecycle.BuildpackGroup, error) {
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
			fmt.Sprintf("%s:%s:", b.WorkspaceVolume, launchDir),
		},
	}, nil, "")
	if err != nil {
		return nil, errors.Wrap(err, "container create")
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
			return nil, errors.Wrap(err, "copy buildpacks to container")
		}

		groups := lifecycle.BuildpackOrder{
			lifecycle.BuildpackGroup{
				Buildpacks: buildpacks,
			},
		}

		var tomlBuilder strings.Builder
		if err := toml.NewEncoder(&tomlBuilder).Encode(map[string]interface{}{"groups": groups}); err != nil {
			return nil, errors.Wrapf(err, "encoding order.toml: %#v", groups)
		}

		orderToml = tomlBuilder.String()
	}

	uid, gid, err := b.packUidGid(b.Builder)
	if err != nil {
		return nil, errors.Wrap(err, "detect")
	}

	tr, errChan := b.FS.CreateTarReader(b.AppDir, launchDir+"/app", uid, gid)
	if err := b.Cli.CopyToContainer(ctx, ctr.ID, "/", tr, dockertypes.CopyToContainerOptions{}); err != nil {
		return nil, errors.Wrap(err, "copy app to workspace volume")
	}
	if err := <-errChan; err != nil {
		return nil, errors.Wrap(err, "copy app to workspace volume")
	}

	if err := b.chownDir(launchDir+"/app", uid, gid); err != nil {
		return nil, errors.Wrap(err, "chown app to workspace volume")
	}

	if orderToml != "" {
		ftr, err := b.FS.CreateSingleFileTar(orderPath, orderToml)
		if err != nil {
			return nil, errors.Wrap(err, "converting order TOML to tar reader")
		}
		if err := b.Cli.CopyToContainer(ctx, ctr.ID, "/", ftr, dockertypes.CopyToContainerOptions{}); err != nil {
			return nil, errors.Wrap(err, fmt.Sprintf("creating %s", orderPath))
		}
	}

	if err := b.Cli.RunContainer(ctx, ctr.ID, b.Stdout, b.Stderr); err != nil {
		return nil, errors.Wrap(err, "run detect container")
	}
	return b.groupToml(ctr.ID)
}

func (b *BuildConfig) groupToml(ctrID string) (*lifecycle.BuildpackGroup, error) {
	trc, _, err := b.Cli.CopyFromContainer(context.Background(), ctrID, groupPath)
	if err != nil {
		return nil, errors.Wrap(err, "reading group.toml from container")
	}
	defer trc.Close()
	tr := tar.NewReader(trc)
	_, err = tr.Next()
	if err != nil {
		return nil, errors.Wrap(err, "extracting group.toml from tar")
	}
	var group lifecycle.BuildpackGroup
	if _, err := toml.DecodeReader(tr, &group); err != nil {
		return nil, errors.Wrap(err, "decoding group.toml")
	}
	return &group, nil
}

func (b *BuildConfig) Analyze() error {
	metadata, err := b.imageLabel(b.RepoName, lifecycle.MetadataLabel, !b.Publish)
	if err != nil {
		return errors.Wrap(err, "analyze image label")
	}
	if metadata == "" {
		if b.Publish {
			b.Log.Printf("WARNING: skipping analyze, image not found or requires authentication to access")
		} else {
			b.Log.Printf("WARNING: skipping analyze, image not found")
		}
		return nil
	}

	ctx := context.Background()
	ctr, err := b.Cli.ContainerCreate(ctx, &container.Config{
		Image: b.Builder,
		Cmd: []string{
			"/lifecycle/analyzer",
			"-launch", launchDir,
			"-group", groupPath,
			"-metadata", launchDir + "/imagemetadata.json",
			b.RepoName,
		},
	}, &container.HostConfig{
		Binds: []string{
			fmt.Sprintf("%s:%s:", b.WorkspaceVolume, launchDir),
		},
	}, nil, "")
	if err != nil {
		return errors.Wrap(err, "analyze container create")
	}
	defer b.Cli.ContainerRemove(ctx, ctr.ID, dockertypes.ContainerRemoveOptions{})

	tr, err := b.FS.CreateSingleFileTar(launchDir+"/imagemetadata.json", metadata)
	if err != nil {
		return errors.Wrap(err, "create tar with image metadata")
	}
	if err := b.Cli.CopyToContainer(ctx, ctr.ID, "/", tr, dockertypes.CopyToContainerOptions{}); err != nil {
		return errors.Wrap(err, "copy image metadata to workspace volume")
	}

	if err := b.Cli.RunContainer(ctx, ctr.ID, b.Stdout, b.Stderr); err != nil {
		return errors.Wrap(err, "analyze run container")
	}
	return nil
}

func (b *BuildConfig) Build() error {
	ctx := context.Background()
	ctr, err := b.Cli.ContainerCreate(ctx, &container.Config{
		Image: b.Builder,
		Cmd: []string{
			"/lifecycle/builder",
			"-buildpacks", buildpacksDir,
			"-launch", launchDir,
			"-cache", cacheDir,
			"-group", groupPath,
			"-plan", planPath,
			"-platform", platformDir,
		},
	}, &container.HostConfig{
		Binds: []string{
			fmt.Sprintf("%s:%s:", b.WorkspaceVolume, launchDir),
			fmt.Sprintf("%s:%s:", b.CacheVolume, cacheDir),
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

	return b.Cli.RunContainer(ctx, ctr.ID, b.Stdout, b.Stderr)
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

func (b *BuildConfig) Export(group *lifecycle.BuildpackGroup) error {
	ctx := context.Background()
	ctr, err := b.Cli.ContainerCreate(ctx, &container.Config{
		Image: b.Builder,
		Cmd: []string{
			"/lifecycle/exporter",
			"-dry-run", "/tmp/pack-exporter",
			"-image", b.RunImage,
			"-launch", launchDir,
			"-group", groupPath,
			b.RepoName,
		},
	}, &container.HostConfig{
		Binds: []string{
			fmt.Sprintf("%s:%s:", b.WorkspaceVolume, launchDir),
		},
	}, nil, "")
	if err != nil {
		return errors.Wrap(err, "export container create")
	}
	defer b.Cli.ContainerRemove(ctx, ctr.ID, dockertypes.ContainerRemoveOptions{})

	if err := b.Cli.RunContainer(ctx, ctr.ID, b.Stdout, b.Stderr); err != nil {
		return errors.Wrap(err, "run lifecycle/exporter")
	}
	defer b.Cli.ContainerRemove(ctx, ctr.ID, dockertypes.ContainerRemoveOptions{})

	r, _, err := b.Cli.CopyFromContainer(ctx, ctr.ID, "/tmp/pack-exporter")
	if err != nil {
		return errors.Wrap(err, "copy from exporter container")
	}
	defer r.Close()

	tmpDir, err := ioutil.TempDir("", "pack.build.")
	if err != nil {
		return errors.Wrap(err, "tmpdir for exporter")
	}
	defer os.RemoveAll(tmpDir)

	if err := b.FS.Untar(r, tmpDir); err != nil {
		return errors.Wrap(err, "untar from exporter container")
	}

	var imgSHA string
	if b.Publish {
		runImageStore, err := img.NewRegistry(b.RunImage)
		if err != nil {
			return errors.Wrap(err, "access")
		}
		runImage, err := runImageStore.Image()
		if err != nil {
			return errors.Wrap(err, "access")
		}

		exporter := &lifecycle.Exporter{
			ArtifactsDir: filepath.Join(tmpDir, "pack-exporter"),
			Buildpacks:   group.Buildpacks,
			Out:          os.Stdout,
			Err:          os.Stderr,
		}
		repoStore, err := img.NewRegistry(b.RepoName)
		if err != nil {
			return errors.Wrap(err, "access")
		}
		origImage, err := repoStore.Image()
		if err != nil {
			return errors.Wrap(err, "access")
		}

		_, err = origImage.ConfigFile()
		if err != nil {
			origImage = nil
		}
		newImage, err := exporter.ExportImage(
			launchDir,
			launchDir+"/app",
			runImage,
			origImage,
		)
		if err != nil {
			return errors.Wrap(err, "export to registry")
		}
		if err := repoStore.Write(newImage); err != nil {
			return errors.Wrap(err, "write")
		}
		hash, err := newImage.Digest()
		if err != nil {
			return errors.Wrap(err, "digest")
		}
		imgSHA = hash.String()
	} else {
		var metadata lifecycle.AppImageMetadata
		bData, err := ioutil.ReadFile(filepath.Join(tmpDir, "pack-exporter", "metadata.json"))

		if err != nil {
			return errors.Wrap(err, "read exporter metadata")
		}
		if err := json.Unmarshal(bData, &metadata); err != nil {
			return errors.Wrap(err, "read exporter metadata")
		}

		// TODO: move to init
		imgFactory, err := image.DefaultFactory()
		if err != nil {
			return errors.Wrap(err, "create default factory")
		}

		img, err := imgFactory.NewLocal(b.RunImage, false)
		if err != nil {
			return errors.Wrap(err, "new local")
		}

		runImageTopLayer, err := img.TopLayer()
		if err != nil {
			return errors.Wrap(err, "get run top layer")
		}
		runImageDigest, err := img.Digest()
		if err != nil {
			return errors.Wrap(err, "get run digest")
		}
		metadata.RunImage = lifecycle.RunImageMetadata{
			TopLayer: runImageTopLayer,
			SHA:      runImageDigest,
		}

		img.Rename(b.RepoName)

		var prevMetadata lifecycle.AppImageMetadata
		if prevInspect, _, err := b.Cli.ImageInspectWithRaw(context.Background(), b.RepoName); err != nil {
			// TODO handle rel error (eg. not prev image not exist)
		} else {
			label := prevInspect.Config.Labels[lifecycle.MetadataLabel]
			if err := json.Unmarshal([]byte(label), &prevMetadata); err != nil {
				return errors.Wrap(err, "parsing previous image metadata label")
			}
		}

		// TODO do alpha sort
		for index, bp := range metadata.Buildpacks {
			var prevBP *lifecycle.BuildpackMetadata
			for _, pbp := range prevMetadata.Buildpacks {
				if pbp.ID == bp.ID {
					prevBP = &pbp
				}
			}

			layerKeys := make([]string, 0, len(bp.Layers))
			for n, _ := range bp.Layers {
				layerKeys = append(layerKeys, n)
			}
			sort.Strings(layerKeys)

			for _, layerName := range layerKeys {
				layer := bp.Layers[layerName]
				if layer.SHA == "" {
					if prevBP == nil {
						return fmt.Errorf("tried to use not exist previous buildpack: %s", bp.ID)
					}
					// TODO error nicely on not found
					layer.SHA = prevBP.Layers[layerName].SHA
					b.Log.Printf("reusing layer '%s/%s' with diffID '%s'\n", bp.ID, layerName, layer.SHA)
					if err := img.ReuseLayer(layer.SHA); err != nil {
						return errors.Wrapf(err, "reuse layer '%s/%s' from previous image", bp.ID, layerName)
					}
					metadata.Buildpacks[index].Layers[layerName] = layer
				} else {
					b.Log.Printf("adding %s layer '%s' with diffID '%s'\n", bp.ID, layerName, layer.SHA)
					if err := img.AddLayer(filepath.Join(tmpDir, "pack-exporter", strings.TrimPrefix(layer.SHA, "sha256:")+".tar")); err != nil {
						return errors.Wrapf(err, "add %s layer '%s'", bp.ID, layerName)
					}
				}
			}
		}

		b.Log.Printf("adding app layer with diffID '%s'\n", metadata.App.SHA)
		if err := img.AddLayer(filepath.Join(tmpDir, "pack-exporter", strings.TrimPrefix(metadata.App.SHA, "sha256:")+".tar")); err != nil {
			return errors.Wrap(err, "add app layer")
		}

		b.Log.Printf("adding config layer with diffID '%s'\n", metadata.Config.SHA)
		if err := img.AddLayer(filepath.Join(tmpDir, "pack-exporter", strings.TrimPrefix(metadata.Config.SHA, "sha256:")+".tar")); err != nil {
			return errors.Wrap(err, "add config layer")
		}

		bData, err = json.Marshal(metadata)
		if err != nil {
			return errors.Wrap(err, "write exporter metadata")
		}
		if err := img.SetLabel(lifecycle.MetadataLabel, string(bData)); err != nil {
			return errors.Wrap(err, "set image metadata label")
		}
		if imgSHA, err = img.Save(); err != nil {
			return errors.Wrap(err, "save image")
		}
	}

	b.Log.Printf("\n*** Image: %s@%s\n", b.RepoName, imgSHA)
	return nil
}

func (b *BuildConfig) imageLabel(repoName, key string, useDaemon bool) (string, error) {
	var labels map[string]string
	if useDaemon {
		i, _, err := b.Cli.ImageInspectWithRaw(context.Background(), repoName)
		if dockercli.IsErrNotFound(err) {
			return "", nil
		} else if err != nil {
			return "", errors.Wrap(err, "analyze read previous image config")
		}
		labels = i.Config.Labels
	} else {
		origImage, err := b.Images.ReadImage(repoName, false)
		if err != nil || origImage == nil {
			return "", err
		}
		config, err := origImage.ConfigFile()
		if err != nil {
			if remoteErr, ok := err.(*remote.Error); ok && len(remoteErr.Errors) > 0 {
				switch remoteErr.Errors[0].Code {
				case remote.UnauthorizedErrorCode, remote.ManifestUnknownErrorCode:
					return "", nil
				}
			}
			return "", errors.Wrapf(err, "access manifest: %s", repoName)
		}
		labels = config.Config.Labels
	}

	return labels[key], nil
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
			fmt.Sprintf("%s:%s:", b.WorkspaceVolume, launchDir),
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

func (b *BuildConfig) exportVolume(image, volName string) (string, func(), error) {
	ctx := context.Background()
	ctr, err := b.Cli.ContainerCreate(ctx, &container.Config{
		Image: b.Builder,
		Cmd:   []string{"true"},
	}, &container.HostConfig{
		Binds: []string{
			fmt.Sprintf("%s:%s:ro", b.WorkspaceVolume, launchDir),
		},
	}, nil, "")
	if err != nil {
		return "", func() {}, errors.Wrap(err, "export container create")
	}
	defer b.Cli.ContainerRemove(ctx, ctr.ID, dockertypes.ContainerRemoveOptions{})

	r, _, err := b.Cli.CopyFromContainer(ctx, ctr.ID, launchDir)
	if err != nil {
		return "", func() {}, err
	}
	defer r.Close()

	tmpDir, err := ioutil.TempDir("", "pack.build.")
	if err != nil {
		return "", func() {}, err
	}
	cleanup := func() { os.RemoveAll(tmpDir) }

	if err := b.FS.Untar(r, tmpDir); err != nil {
		cleanup()
		return "", func() {}, err
	}

	return filepath.Join(tmpDir, "workspace"), cleanup, nil
}
func randString(n int) string {
	b := make([]byte, n)
	for i := range b {
		b[i] = 'a' + byte(rand.Intn(26))
	}
	return string(b)
}
