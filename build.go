package pack

import (
	"archive/tar"
	"context"
	"crypto/md5"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"math/rand"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/BurntSushi/toml"
	"github.com/buildpack/lifecycle"
	dockertypes "github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	dockercli "github.com/docker/docker/client"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/google/uuid"
	"github.com/pkg/errors"

	"github.com/buildpack/pack/config"
	"github.com/buildpack/pack/docker"
	"github.com/buildpack/pack/fs"
	"github.com/buildpack/pack/image"
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
	RepoName   string
	Publish    bool
	NoPull     bool
	Buildpacks []string
}

type BuildConfig struct {
	AppDir     string
	Builder    string
	RunImage   string
	RepoName   string
	Publish    bool
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

const defaultLaunchDir = "/workspace"

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

	f.Config, err = config.New(filepath.Join(os.Getenv("HOME"), ".pack"))
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
	if !f.NoPull {
		bf.Log.Printf("Pulling builder image '%s' (use --no-pull flag to skip this step)", f.Builder)
		if err := bf.Cli.PullImage(f.Builder); err != nil {
			return nil, err
		}
	}

	b := &BuildConfig{
		AppDir:          appDir,
		Builder:         f.Builder,
		RepoName:        f.RepoName,
		Publish:         f.Publish,
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

	builderStackID, err := b.imageLabel(f.Builder, "io.buildpacks.stack.id", true)
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
		bf.Log.Printf("Using user provided run image '%s'", f.RunImage)
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

func (b *BuildConfig) Detect() (*lifecycle.BuildpackGroup, error) {
	var orderToml string
	if len(b.Buildpacks) == 0 {
		fmt.Println("*** DETECTING:")
		orderToml = "" // use order toml already in image
	} else {
		fmt.Println("*** DETECTING WITH MANUALLY-PROVIDED GROUP:")
		var order struct {
			Groups lifecycle.BuildpackOrder `toml:"groups"`
		}
		order.Groups = lifecycle.BuildpackOrder{
			lifecycle.BuildpackGroup{
				Buildpacks: []*lifecycle.Buildpack{},
			},
		}

		for _, bp := range b.Buildpacks {
			id, version := parseBuildpack(bp)
			order.Groups[0].Buildpacks = append(
				order.Groups[0].Buildpacks,
				&lifecycle.Buildpack{ID: id, Version: version, Optional: false},
			)
		}

		tomlBuilder := &strings.Builder{}
		err := toml.NewEncoder(tomlBuilder).Encode(order)
		if err != nil {
			return nil, errors.Wrapf(err, "encoding order.toml: %#v", order)
		}

		orderToml = tomlBuilder.String()
	}

	var cmd []string
	if orderToml != "" {
		cmd = []string{"/lifecycle/detector", "-order", "/workspace/app/pack-order.toml"}
	} else {
		cmd = []string{"/lifecycle/detector"}
	}

	ctx := context.Background()
	ctr, err := b.Cli.ContainerCreate(ctx, &container.Config{
		Image: b.Builder,
		Cmd:   cmd,
	}, &container.HostConfig{
		Binds: []string{
			b.WorkspaceVolume + ":/workspace",
		},
	}, nil, "")
	if err != nil {
		return nil, errors.Wrap(err, "container create")
	}
	defer b.Cli.ContainerRemove(ctx, ctr.ID, dockertypes.ContainerRemoveOptions{})

	uid, gid, err := b.packUidGid(b.Builder)
	if err != nil {
		return nil, errors.Wrap(err, "detect")
	}

	tr, errChan := b.FS.CreateTarReader(b.AppDir, "/workspace/app", uid, gid)
	if err := b.Cli.CopyToContainer(ctx, ctr.ID, "/", tr, dockertypes.CopyToContainerOptions{}); err != nil {
		return nil, errors.Wrap(err, "copy app to workspace volume")
	}
	if err := <-errChan; err != nil {
		return nil, errors.Wrap(err, "copy app to workspace volume")
	}

	if err := b.chownDir("/workspace/app", uid, gid); err != nil {
		return nil, errors.Wrap(err, "chown app to workspace volume")
	}

	if orderToml != "" {
		ftr, err := b.FS.CreateSingleFileTar("/workspace/app/pack-order.toml", orderToml)
		if err != nil {
			return nil, errors.Wrap(err, "converting order TOML to tar reader")
		}
		if err := b.Cli.CopyToContainer(ctx, ctr.ID, "/", ftr, dockertypes.CopyToContainerOptions{}); err != nil {
			return nil, errors.Wrap(err, "creating /workspace/app/pack-order.toml")
		}
	}

	if err := b.Cli.RunContainer(ctx, ctr.ID, b.Stdout, b.Stderr); err != nil {
		return nil, errors.Wrap(err, "run detect container")
	}
	return b.groupToml(ctr.ID)
}

func (b *BuildConfig) groupToml(ctrID string) (*lifecycle.BuildpackGroup, error) {
	trc, _, err := b.Cli.CopyFromContainer(context.Background(), ctrID, "/workspace/group.toml")
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
		Cmd:   []string{"/lifecycle/analyzer", "-metadata", "/workspace/imagemetadata.json", "-launch", "/workspace", b.RepoName},
	}, &container.HostConfig{
		Binds: []string{
			b.WorkspaceVolume + ":/workspace",
		},
	}, nil, "")
	if err != nil {
		return errors.Wrap(err, "analyze container create")
	}
	defer b.Cli.ContainerRemove(ctx, ctr.ID, dockertypes.ContainerRemoveOptions{})

	tr, err := b.FS.CreateSingleFileTar("/workspace/imagemetadata.json", metadata)
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
		Cmd:   []string{"/lifecycle/builder"},
	}, &container.HostConfig{
		Binds: []string{
			b.WorkspaceVolume + ":/workspace",
			b.CacheVolume + ":/cache",
		},
	}, nil, "")
	if err != nil {
		return errors.Wrap(err, "build container create")
	}
	defer b.Cli.ContainerRemove(ctx, ctr.ID, dockertypes.ContainerRemoveOptions{})

	return b.Cli.RunContainer(ctx, ctr.ID, b.Stdout, b.Stderr)
}

func (b *BuildConfig) Export(group *lifecycle.BuildpackGroup) error {
	uid, gid, err := b.packUidGid(b.Builder)
	if err != nil {
		return errors.Wrap(err, "export")
	}

	if b.Publish {
		localWorkspaceDir, cleanup, err := b.exportVolume(b.Builder, b.WorkspaceVolume)
		if err != nil {
			return err
		}
		defer cleanup()

		imgSHA, err := exportRegistry(group, uid, gid, localWorkspaceDir, b.RepoName, b.RunImage, b.Stdout, b.Stderr)
		if err != nil {
			return err
		}
		b.Log.Printf("\n*** Image: %s@%s\n", b.RepoName, imgSHA)
	} else {
		var buildpacks []string
		for _, b := range group.Buildpacks {
			buildpacks = append(buildpacks, b.ID)
		}

		if err := exportDaemon(b.Cli, buildpacks, b.WorkspaceVolume, b.RepoName, b.RunImage, b.Stdout, uid, gid); err != nil {
			return err
		}
	}

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
		} else if len(kv2) == 2 && kv2[0] == "PACK_USER_GID" {
			if sGID == "" {
				sGID = kv2[1]
			}
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
			b.WorkspaceVolume + ":/workspace",
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
			b.WorkspaceVolume + ":/workspace:ro",
		},
	}, nil, "")
	if err != nil {
		return "", func() {}, errors.Wrap(err, "export container create")
	}
	defer b.Cli.ContainerRemove(ctx, ctr.ID, dockertypes.ContainerRemoveOptions{})

	r, _, err := b.Cli.CopyFromContainer(ctx, ctr.ID, "/workspace")
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
