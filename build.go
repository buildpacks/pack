package pack

import (
	"archive/tar"
	"context"
	"crypto/md5"
	"fmt"
	"io"
	"io/ioutil"
	"log"
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
	// Below are set by init
	Cli             *docker.Docker
	WorkspaceVolume string
	CacheVolume     string
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

	return nil
}

func (b *BuildFlags) Close() error {
	return b.Cli.VolumeRemove(context.Background(), b.WorkspaceVolume, true)
}

func (b *BuildFlags) Run() error {
	fmt.Println("*** PULLING BUILDER IMAGE LOCALLY:")
	if err := b.PullImage(b.Builder); err != nil {
		return errors.Wrapf(err, "pull image: %s", b.Builder)
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

	if !b.Publish {
		fmt.Println("*** PULLING RUN IMAGE LOCALLY:")
		if err := b.PullImage(b.RunImage); err != nil {
			return errors.Wrapf(err, "pull image: %s", b.RunImage)
		}
	}

	fmt.Println("*** EXPORTING:")
	if b.Publish {
		localWorkspaceDir, cleanup, err := b.exportVolume(b.Builder, b.WorkspaceVolume)
		if err != nil {
			return err
		}
		defer cleanup()

		imgSHA, err := exportRegistry(group, localWorkspaceDir, b.RepoName, b.RunImage)
		if err != nil {
			return err
		}
		fmt.Printf("\n*** Image: %s@%s\n", b.RepoName, imgSHA)
	} else {
		var buildpacks []string
		for _, b := range group.Buildpacks {
			buildpacks = append(buildpacks, b.ID)
		}

		if err := exportDaemon(b.Cli, buildpacks, b.WorkspaceVolume, b.RepoName, b.RunImage); err != nil {
			return err
		}
	}

	return nil
}

func (b *BuildFlags) PullImage(ref string) error {
	rc, err := b.Cli.ImagePull(context.Background(), ref, dockertypes.ImagePullOptions{})
	if err != nil {
		return err
	}
	if _, err := io.Copy(ioutil.Discard, rc); err != nil {
		return err
	}
	return rc.Close()
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

	tr, errChan := createTarReader(b.AppDir, "/workspace/app")
	if err := b.Cli.CopyToContainer(ctx, ctr.ID, "/", tr, dockertypes.CopyToContainerOptions{}); err != nil {
		return nil, errors.Wrap(err, "copy app to workspace volume")
	}
	if err := <-errChan; err != nil {
		return nil, errors.Wrap(err, "copy app to workspace volume")
	}

	if err := b.Cli.RunContainer(ctx, ctr.ID, os.Stdout, os.Stderr); err != nil {
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
		return err
	}
	if metadata == "" {
		return nil
	}

	ctx := context.Background()
	ctr, err := b.Cli.ContainerCreate(ctx, &container.Config{
		Image: b.Builder,
		Cmd:   []string{"/lifecycle/analyzer", "-metadata", metadata, "-launch", "/workspace", b.RepoName},
	}, &container.HostConfig{
		Binds: []string{
			b.WorkspaceVolume + ":/workspace",
		},
	}, nil, "")
	if err != nil {
		return errors.Wrap(err, "analyze container create")
	}
	defer b.Cli.ContainerRemove(ctx, ctr.ID, dockertypes.ContainerRemoveOptions{})

	return b.Cli.RunContainer(ctx, ctr.ID, os.Stdout, os.Stderr)
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

	return b.Cli.RunContainer(ctx, ctr.ID, os.Stdout, os.Stderr)
}

func (b *BuildFlags) imageLabel(key string) (string, error) {
	if b.Publish {
		repoStore, err := img.NewRegistry(b.RepoName)
		if err != nil {
			log.Printf("WARNING: skipping analyze, image not found or requires authentication to access: %s", err)
			return "", nil
		}
		origImage, err := repoStore.Image()
		if err != nil {
			log.Printf("WARNING: skipping analyze, image not found or requires authentication to access: %s", err)
			return "", nil
		}
		config, err := origImage.ConfigFile()
		if err != nil {
			if remoteErr, ok := err.(*remote.Error); ok && len(remoteErr.Errors) > 0 {
				switch remoteErr.Errors[0].Code {
				case remote.UnauthorizedErrorCode, remote.ManifestUnknownErrorCode:
					log.Printf("WARNING: skipping analyze, image not found or requires authentication to access: %s", remoteErr)
					return "", nil
				}
			}
			return "", errors.Wrapf(err, "access manifest: %s", b.RepoName)
		}
		return config.Config.Labels[key], nil
	}

	i, _, err := b.Cli.ImageInspectWithRaw(context.Background(), b.RepoName)
	if dockercli.IsErrNotFound(err) {
		log.Printf("WARNING: skipping analyze, image not found")
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

	if err := Untar(r, tmpDir); err != nil {
		cleanup()
		return "", func() {}, err
	}

	return filepath.Join(tmpDir, "workspace"), cleanup, nil
}
