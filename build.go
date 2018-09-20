package pack

import (
	"archive/tar"
	"context"
	"crypto/md5"
	"fmt"
	"github.com/buildpack/pack/fs"
	"io"
	"io/ioutil"
	"log"
	"math/rand"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
	"github.com/buildpack/lifecycle"
	"github.com/buildpack/pack/docker"
	"github.com/buildpack/packs/img"
	dockertypes "github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	dockercli "github.com/docker/docker/client"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/google/uuid"
	"github.com/pkg/errors"
)

func Build(appDir, buildImage, runImage, repoName string, publish bool) error {
	b := &BuildFlags{
		AppDir:   appDir,
		Builder:  buildImage,
		RunImage: runImage,
		RepoName: repoName,
		Publish:  publish,
	}
	if err := b.Init(); err != nil {
		return err
	}
	defer b.Close()
	return b.Run()
}

type BuildFlags struct {
	AppDir   string
	Builder  string
	RunImage string
	RepoName string
	Publish  bool
	NoPull   bool
	// Below are set by init
	Cli             *docker.Docker
	WorkspaceVolume string
	CacheVolume     string
	Stdout          io.Writer
	Stderr          io.Writer
	Log             *log.Logger
	FS              FS
}

func (b *BuildFlags) Init() error {
	var err error
	b.AppDir, err = filepath.Abs(b.AppDir)
	if err != nil {
		return err
	}

	b.Cli, err = docker.New()
	if err != nil {
		return err
	}

	b.WorkspaceVolume = fmt.Sprintf("pack-workspace-%x", uuid.New().String())
	b.CacheVolume = fmt.Sprintf("pack-cache-%x", md5.Sum([]byte(b.AppDir)))

	b.Stdout = os.Stdout
	b.Stderr = os.Stderr
	b.Log = log.New(os.Stdout, "", log.LstdFlags|log.Lshortfile)
	b.FS = &fs.FS{}

	return nil
}

func (b *BuildFlags) Close() error {
	return b.Cli.VolumeRemove(context.Background(), b.WorkspaceVolume, true)
}

func (b *BuildFlags) Run() error {
	if !b.NoPull {
		fmt.Println("*** PULLING BUILDER IMAGE LOCALLY:")
		if err := b.Cli.PullImage(b.Builder); err != nil {
			return errors.Wrapf(err, "pull image: %s", b.Builder)
		}
	}

	fmt.Println("*** DETECTING:")
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

	if !b.Publish && !b.NoPull {
		fmt.Println("*** PULLING RUN IMAGE LOCALLY:")
		if err := b.Cli.PullImage(b.RunImage); err != nil {
			return errors.Wrapf(err, "pull image: %s", b.RunImage)
		}
	}

	fmt.Println("*** EXPORTING:")
	if err := b.Export(group); err != nil {
		return err
	}

	return nil
}

func (b *BuildFlags) Detect() (*lifecycle.BuildpackGroup, error) {
	ctx := context.Background()
	ctr, err := b.Cli.ContainerCreate(ctx, &container.Config{
		Image: b.Builder,
		Cmd:   []string{"/lifecycle/detector"},
	}, &container.HostConfig{
		Binds: []string{
			b.WorkspaceVolume + ":/workspace",
		},
	}, nil, "")
	if err != nil {
		return nil, errors.Wrap(err, "container create")
	}
	defer b.Cli.ContainerRemove(ctx, ctr.ID, dockertypes.ContainerRemoveOptions{})

	tr, errChan := b.FS.CreateTarReader(b.AppDir, "/workspace/app")
	if err := b.Cli.CopyToContainer(ctx, ctr.ID, "/", tr, dockertypes.CopyToContainerOptions{}); err != nil {
		return nil, errors.Wrap(err, "copy app to workspace volume")
	}
	if err := <-errChan; err != nil {
		return nil, errors.Wrap(err, "copy app to workspace volume")
	}

	if err := b.Cli.RunContainer(ctx, ctr.ID, b.Stdout, b.Stderr); err != nil {
		return nil, errors.Wrap(err, "run detect container")
	}
	return b.groupToml(ctr.ID)
}

func (b *BuildFlags) groupToml(ctrID string) (*lifecycle.BuildpackGroup, error) {
	trc, _, err := b.Cli.CopyFromContainer(context.Background(), ctrID, "/workspace/group.toml")
	if err != nil {
		return nil, errors.Wrap(err, "reading group.toml from container")
	}
	defer trc.Close()
	tr := tar.NewReader(trc)
	_, err = tr.Next()
	if err != nil {
		return nil, errors.Wrap(err, "reading group.toml from container")
	}
	var group lifecycle.BuildpackGroup
	if _, err := toml.DecodeReader(tr, &group); err != nil {
		return nil, errors.Wrap(err, "reading group.toml from container")
	}
	return &group, nil
}

func (b *BuildFlags) Analyze() error {
	metadata, err := b.imageLabel(lifecycle.MetadataLabel)
	if err != nil {
		return errors.Wrap(err, "analyze image label")
	}
	if metadata == "" {
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

func (b *BuildFlags) Build() error {
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

func (b *BuildFlags) Export(group *lifecycle.BuildpackGroup) error {
	if b.Publish {
		localWorkspaceDir, cleanup, err := b.exportVolume(b.Builder, b.WorkspaceVolume)
		if err != nil {
			return err
		}
		defer cleanup()

		imgSHA, err := exportRegistry(group, localWorkspaceDir, b.RepoName, b.RunImage, b.Stdout, b.Stderr)
		if err != nil {
			return err
		}
		b.Log.Printf("\n*** Image: %s@%s\n", b.RepoName, imgSHA)
	} else {
		var buildpacks []string
		for _, b := range group.Buildpacks {
			buildpacks = append(buildpacks, b.ID)
		}

		if err := exportDaemon(b.Cli, buildpacks, b.WorkspaceVolume, b.RepoName, b.RunImage, b.Stdout); err != nil {
			return err
		}
	}

	return nil
}

func (b *BuildFlags) imageLabel(key string) (string, error) {
	if b.Publish {
		repoStore, err := img.NewRegistry(b.RepoName)
		if err != nil {
			b.Log.Printf("WARNING: skipping analyze, image not found or requires authentication to access: %s", err)
			return "", nil
		}
		origImage, err := repoStore.Image()
		if err != nil {
			b.Log.Printf("WARNING: skipping analyze, image not found or requires authentication to access: %s", err)
			return "", nil
		}
		config, err := origImage.ConfigFile()
		if err != nil {
			if remoteErr, ok := err.(*remote.Error); ok && len(remoteErr.Errors) > 0 {
				switch remoteErr.Errors[0].Code {
				case remote.UnauthorizedErrorCode, remote.ManifestUnknownErrorCode:
					b.Log.Printf("WARNING: skipping analyze, image not found or requires authentication to access: %s", remoteErr)
					return "", nil
				}
			}
			return "", errors.Wrapf(err, "access manifest: %s", b.RepoName)
		}
		return config.Config.Labels[key], nil
	}

	i, _, err := b.Cli.ImageInspectWithRaw(context.Background(), b.RepoName)
	if dockercli.IsErrNotFound(err) {
		b.Log.Printf("WARNING: skipping analyze, image not found")
		return "", nil
	} else if err != nil {
		return "", errors.Wrap(err, "analyze read previous image config")
	}
	return i.Config.Labels[key], nil
}

func (b *BuildFlags) exportVolume(image, volName string) (string, func(), error) {
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
