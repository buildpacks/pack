package image

import (
	"fmt"

	"github.com/buildpack/lifecycle/img"
	"github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/mutate"
	"github.com/google/go-containerregistry/pkg/v1/types"
	"github.com/pkg/errors"
)

type remote struct {
	RepoName string
	Image    v1.Image
}

func (f *Factory) NewRemote(repoName string) (Image, error) {
	repoStore, err := img.NewRegistry(repoName)
	if err != nil {
		return nil, err
	}
	image, err := repoStore.Image()
	if err != nil {
		return nil, errors.New("connect to repo store")
	}

	return &remote{
		RepoName: repoName,
		Image:    image,
	}, nil
}

func (r *remote) Label(key string) (string, error) {
	cfg, err := r.Image.ConfigFile()
	if err != nil || cfg == nil {
		return "", fmt.Errorf("failed to get label, image '%s' does not exist", r.RepoName)
	}
	labels := cfg.Config.Labels
	return labels[key], nil

}

func (r *remote) Rename(name string) {
	r.RepoName = name
}

func (r *remote) Name() string {
	return r.RepoName
}

func (r *remote) Digest() (string, error) {
	hash, err := r.Image.Digest()
	if err != nil {
		return "", fmt.Errorf("failed to get digest for image '%s': %s", r.RepoName, err)
	}
	return hash.String(), nil
}

func (r *remote) Rebase(baseTopLayer string, newBase Image) error {
	newBaseRemote, ok := newBase.(*remote)
	if !ok {
		return errors.New("expected new base to be a remote image")
	}

	oldBase := &subImage{img: r.Image, topSHA: baseTopLayer}
	newImage, err := mutate.Rebase(r.Image, oldBase, newBaseRemote.Image, &mutate.RebaseOptions{})
	if err != nil {
		return errors.Wrap(err, "rebase")
	}
	r.Image = newImage
	return nil
}

func (r *remote) SetLabel(key, val string) error {
	newImage, err := img.Label(r.Image, key, val)
	if err != nil {
		return errors.Wrap(err, "set metadata label")
	}
	r.Image = newImage
	return nil
}

func (r *remote) TopLayer() (string, error) {
	all, err := r.Image.Layers()
	if err != nil {
		return "", err
	}
	topLayer := all[len(all)-1]
	hex, err := topLayer.DiffID()
	if err != nil {
		return "", err
	}
	return hex.String(), nil
}

func (r *remote) AddLayer(path string) error {
	panic("Not Implemented")
}

func (r *remote) ReuseLayer(sha string) error {
	panic("Not Implemented")
}

func (r *remote) Save() (string, error) {
	repoStore, err := img.NewRegistry(r.RepoName)
	if err != nil {
		return "", err
	}
	if err := repoStore.Write(r.Image); err != nil {
		return "", err
	}

	hex, err := r.Image.Digest()

	return hex.String(), nil
}

type subImage struct {
	img    v1.Image
	topSHA string
}

func (si *subImage) Layers() ([]v1.Layer, error) {
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
func (si *subImage) BlobSet() (map[v1.Hash]struct{}, error)  { panic("Not Implemented") }
func (si *subImage) MediaType() (types.MediaType, error)     { panic("Not Implemented") }
func (si *subImage) ConfigName() (v1.Hash, error)            { panic("Not Implemented") }
func (si *subImage) ConfigFile() (*v1.ConfigFile, error)     { panic("Not Implemented") }
func (si *subImage) RawConfigFile() ([]byte, error)          { panic("Not Implemented") }
func (si *subImage) Digest() (v1.Hash, error)                { panic("Not Implemented") }
func (si *subImage) Manifest() (*v1.Manifest, error)         { panic("Not Implemented") }
func (si *subImage) RawManifest() ([]byte, error)            { panic("Not Implemented") }
func (si *subImage) LayerByDigest(v1.Hash) (v1.Layer, error) { panic("Not Implemented") }
func (si *subImage) LayerByDiffID(v1.Hash) (v1.Layer, error) { panic("Not Implemented") }
