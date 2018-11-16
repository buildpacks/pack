package image

import (
	"context"
	"io"
	"log"
	"os"

	"github.com/buildpack/lifecycle/img"
	"github.com/buildpack/pack/docker"
	"github.com/buildpack/pack/fs"
	"github.com/buildpack/packs"
	"github.com/docker/docker/api/types"
	"github.com/google/go-containerregistry/pkg/v1"
)

type Image interface {
	Label(string) (string, error)
	Rename(name string)
	Name() string
	Digest() (string, error)
	Rebase(string, Image) error
	SetLabel(string, string) error
	TopLayer() (string, error)
	AddLayer(path string) error
	ReuseLayer(sha string) error
	Save() (string, error)
}

type Docker interface {
	PullImage(ref string) error
	ImageInspectWithRaw(ctx context.Context, imageID string) (types.ImageInspect, []byte, error)
	ImageBuild(ctx context.Context, buildContext io.Reader, options types.ImageBuildOptions) (types.ImageBuildResponse, error)
	ImageRemove(ctx context.Context, ref string, options types.ImageRemoveOptions) ([]types.ImageDeleteResponseItem, error)
	ImageLoad(ctx context.Context, r io.Reader, quiet bool) (types.ImageLoadResponse, error)
	ImageSave(ctx context.Context, imageIDs []string) (io.ReadCloser, error)
}

type Factory struct {
	Docker Docker
	Log    *log.Logger
	Stdout io.Writer
	FS     *fs.FS
}

func DefaultFactory() (*Factory, error) {
	f := &Factory{
		Stdout: os.Stdout,
		Log:    log.New(os.Stdout, "", log.LstdFlags),
		FS:     &fs.FS{},
	}

	var err error
	f.Docker, err = docker.New()
	if err != nil {
		return nil, err
	}

	return f, nil
}

type Client struct{}

func (c *Client) ReadImage(repoName string, useDaemon bool) (v1.Image, error) {
	repoStore, err := c.RepoStore(repoName, useDaemon)
	if err != nil {
		return nil, err
	}

	origImage, err := repoStore.Image()
	if err != nil {
		// Assume error is due to non-existent image
		return nil, nil
	}
	if _, err := origImage.RawManifest(); err != nil {
		// Assume error is due to non-existent image
		// This is necessary for registries
		return nil, nil
	}

	return origImage, nil
}

func (c *Client) RepoStore(repoName string, useDaemon bool) (img.Store, error) {
	newRepoStore := img.NewRegistry
	if useDaemon {
		newRepoStore = img.NewDaemon
	}
	repoStore, err := newRepoStore(repoName)
	if err != nil {
		return nil, packs.FailErr(err, "access", repoName)
	}
	return repoStore, nil
}
