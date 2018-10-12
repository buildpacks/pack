package pack

import (
	"context"
	"encoding/json"
	"fmt"
	"log"

	"github.com/buildpack/pack/config"
	dockercli "github.com/docker/docker/client"
	"github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/mutate"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/google/go-containerregistry/pkg/v1/types"
	"github.com/pkg/errors"
)

type RebaseConfig struct {
	RepoName  string
	Publish   bool
	Repo      WritableStore
	RepoImage v1.Image
	OldBase   v1.Image
	NewBase   v1.Image
}

type WritableStore interface {
	Write(image v1.Image) error
}

type RebaseFactory struct {
	Log    *log.Logger
	Docker Docker
	Config *config.Config
	Images Images
}

type RebaseFlags struct {
	RepoName string
	Publish  bool
	NoPull   bool
}

func (f *RebaseFactory) RebaseConfigFromFlags(flags RebaseFlags) (RebaseConfig, error) {
	if !flags.NoPull && !flags.Publish {
		f.Log.Println("Pulling image", flags.RepoName)
		err := f.Docker.PullImage(flags.RepoName)
		if err != nil {
			return RebaseConfig{}, fmt.Errorf(`failed to pull stack build image "%s": %s`, flags.RepoName, err)
		}
	}
	stackID, err := f.imageLabel(flags.RepoName, "io.buildpacks.stack.id", !flags.Publish)
	if err != nil {
		return RebaseConfig{}, err
	}

	baseImageName, err := f.baseImageName(stackID, flags.RepoName)
	if err != nil {
		return RebaseConfig{}, err
	}
	if !flags.NoPull && !flags.Publish {
		f.Log.Println("Pulling base image", baseImageName)
		err := f.Docker.PullImage(baseImageName)
		if err != nil {

			return RebaseConfig{}, fmt.Errorf(`failed to pull stack build image "%s": %s`, baseImageName, err)
		}
	}

	repoStore, err := f.Images.RepoStore(flags.RepoName, !flags.Publish)
	if err != nil {
		return RebaseConfig{}, fmt.Errorf(`failed to create repository store for image "%s": %s`, flags.RepoName, err)
	}

	f.Log.Println("Reading image", flags.RepoName)
	repoImage, err := f.Images.ReadImage(flags.RepoName, !flags.Publish)
	if err != nil {
		return RebaseConfig{}, fmt.Errorf(`failed to read image "%s": %s`, flags.RepoName, err)
	}
	oldBase, err := f.fakeBaseImage(flags.RepoName, repoImage, !flags.Publish)
	if err != nil {
		return RebaseConfig{}, fmt.Errorf(`failed to read old base image "%s": %s`, baseImageName, err)
	}
	f.Log.Println("Reading new base image", baseImageName)
	newBase, err := f.Images.ReadImage(baseImageName, !flags.Publish)
	if err != nil {
		return RebaseConfig{}, fmt.Errorf(`failed to read new base image "%s": %s`, baseImageName, err)
	}

	return RebaseConfig{
		RepoName:  flags.RepoName,
		Publish:   flags.Publish,
		Repo:      repoStore,
		RepoImage: repoImage,
		OldBase:   oldBase,
		NewBase:   newBase,
	}, nil
}

func (f *RebaseFactory) Rebase(cfg RebaseConfig) error {
	newImage, err := mutate.Rebase(cfg.RepoImage, cfg.OldBase, cfg.NewBase, &mutate.RebaseOptions{})
	if err != nil {
		return err
	}

	// TODO : set runimage/sha on image metadata
	if err := f.setRunImageSHA(newImage, cfg.NewBase); err != nil {
		return err
	}

	h, err := newImage.Digest()
	if err != nil {
		return err
	}

	// TODO write image
	if err := cfg.Repo.Write(newImage); err != nil {
		return err
	}

	// TODO make sure hash is correct (I think it is currently wrong)
	f.Log.Printf("Successfully replaced %s with %s\n", cfg.RepoName, h)
	return nil
}

// TODO copied from create_builder.go
func (f *RebaseFactory) baseImageName(stackID, repoName string) (string, error) {
	stack, err := f.Config.Get(stackID)
	if err != nil {
		return "", err
	}
	if len(stack.RunImages) == 0 {
		return "", fmt.Errorf(`Invalid stack: stack "%s" requies at least one build image`, stack.ID)
	}
	registry, err := config.Registry(repoName)
	if err != nil {
		return "", err
	}
	return config.ImageByRegistry(registry, stack.RunImages)
}

// TODO copied from build.go
func (f *RebaseFactory) imageLabel(repoName, key string, useDaemon bool) (string, error) {
	var labels map[string]string
	if useDaemon {
		i, _, err := f.Docker.ImageInspectWithRaw(context.Background(), repoName)
		if dockercli.IsErrNotFound(err) {
			return "", nil
		} else if err != nil {
			return "", errors.Wrap(err, "analyze read previous image config")
		}
		labels = i.Config.Labels
	} else {
		origImage, err := f.Images.ReadImage(repoName, false)
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

func (f *RebaseFactory) setRunImageSHA(img, runImage v1.Image) error {
	layers, err := runImage.Layers()
	if err != nil {
		return err
	}
	topSHA, err := layers[len(layers)-1].DiffID()
	if err != nil {
		return err
	}

	cfg, err := img.ConfigFile()
	if err != nil {
		return err
	}
	var metadata map[string]interface{}
	if err = json.Unmarshal([]byte(cfg.Config.Labels["io.buildpacks.lifecycle.metadata"]), &metadata); err != nil {
		return err
	}
	metadata["runimage"] = map[string]string{"sha": topSHA.String()}
	b, err := json.Marshal(metadata)
	if err != nil {
		return err
	}
	cfg.Config.Labels["io.buildpacks.lifecycle.metadata"] = string(b)

	return nil
}

func (f *RebaseFactory) fakeBaseImage(repoName string, repoImage v1.Image, useDaemon bool) (v1.Image, error) {
	str, err := f.imageLabel(repoName, "io.buildpacks.lifecycle.metadata", useDaemon)
	if err != nil {
		return nil, err
	}
	var metadata struct {
		RunImage struct {
			SHA string `toml:"sha"`
		} `toml:"runimage"`
	}
	if err := json.Unmarshal([]byte(str), &metadata); err != nil {
		return nil, err
	}

	return &SubImage{img: repoImage, topSHA: metadata.RunImage.SHA}, nil
}

type SubImage struct {
	img    v1.Image
	topSHA string
}

func (si *SubImage) Layers() ([]v1.Layer, error) {
	all, err := si.img.Layers()
	if err != nil {
		return nil, err
	}
	for i, l := range all {
		d, err := l.DiffID()
		if err != nil {
			return nil, err
		}
		if d.String() == si.topSHA {
			return all[:i+1], nil
		}
	}
	return nil, errors.New("could not find base layer in image")
}
func (si *SubImage) BlobSet() (map[v1.Hash]struct{}, error)  { panic("Not Implemented") }
func (si *SubImage) MediaType() (types.MediaType, error)     { panic("Not Implemented") }
func (si *SubImage) ConfigName() (v1.Hash, error)            { panic("Not Implemented") }
func (si *SubImage) ConfigFile() (*v1.ConfigFile, error)     { panic("Not Implemented") }
func (si *SubImage) RawConfigFile() ([]byte, error)          { panic("Not Implemented") }
func (si *SubImage) Digest() (v1.Hash, error)                { panic("Not Implemented") }
func (si *SubImage) Manifest() (*v1.Manifest, error)         { panic("Not Implemented") }
func (si *SubImage) RawManifest() ([]byte, error)            { panic("Not Implemented") }
func (si *SubImage) LayerByDigest(v1.Hash) (v1.Layer, error) { panic("Not Implemented") }
func (si *SubImage) LayerByDiffID(v1.Hash) (v1.Layer, error) { panic("Not Implemented") }
