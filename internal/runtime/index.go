package runtime

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/buildpacks/imgutil"
	"github.com/google/go-containerregistry/pkg/name"
	v1 "github.com/google/go-containerregistry/pkg/v1"
)

type ImageIndex struct {
	Index     imgutil.Index
	Instances map[string][]ManifestConfig `json:"instance"`
	runtime   *runtime
}

type ManifestConfig struct {
	Hash         v1.Hash            `json:"hash"`
	OS           string             `json:"os,omitempty"`
	OSVersion    string             `json:"os-version,omitempty"`
	Variant      string             `json:"variant,omitempty"`
	Architecture string             `json:"arch,omitempty"`
	MediaType    imgutil.MediaTypes `json:"mediaType,omitempty"`
	Local        bool               `json:"local,omitempty"`
}

type PushOptions struct {
	ManifestType string
	All          bool
	Insecure     bool
}

type imageIndex interface {
	Save(names []string, mimeType string) (string, error)
	Push(ctx context.Context, opts PushOptions) (name.Digest, error)
	Add(ctx context.Context, ref name.Reference, all bool) (name.Digest, error)
	Remove(digest name.Digest) error
	Delete(name string) error
}

func (i *ImageIndex) Save(names []string, mimeType string) (err error) {
	for _, name := range names {
		ref, err := i.runtime.ParseReference(name)
		if err != nil {
			return err
		}
		writeManifest := func(path string) error {
			manifest, err := json.MarshalIndent(i.Instances[ref.Identifier()], "", "    ")
			file, err := os.Create(path)
			if err != nil {
				return err
			}
			_, err = file.Write(manifest)
			if err != nil {
				return err
			}
			return nil
		}
		path := filepath.Join(i.runtime.manifestListPath, makeFilesafeName(ref.Identifier()))
		if _, err := os.Stat(filepath.Join(path, makeFilesafeName(name)+".config.json")); err != nil {
			fmt.Printf("overriding '%s'...", ref.Name())
			if err := writeManifest(filepath.Join(path, makeFilesafeName(name)+".config.json")); err != nil {
				return err
			}
		}
		if err := writeManifest(filepath.Join(path, makeFilesafeName(name)+".config.json")); err != nil {
			return err
		}
		i.Index.Save(path, name, i.runtime.ImageType(mimeType))
	}
	return nil
}

func (i *ImageIndex) Push(ctx context.Context, opts PushOptions) (digest name.Digest, err error) {
	for _, manifest := range i.Index.Manifests {
		for k, v := range i.Instances {
			for _, m := range v {
				if m.Hash.String() == manifest.Digest.String() && m.Local {
					fmt.Errorf("image: '%s' is not found in registry", k)
				}
			}
		}
	}
	digest, err = i.Index.Push(ctx, opts)
	if err == nil {
		fmt.Printf("successfully pushed ImageIndex to registry")
	}
	return
}

func (i *ImageIndex) Add(ctx context.Context, ref name.Reference, all bool) (digest name.Digest, err error) {
	for _, v := range i.Instances[ref.Identifier()] {
		var img string
		if image, err := i.runtime.ParseReference(ref.Name()); err == nil {
			img = strings.Split(image.Name(), ":")[0] + "@" + v.Hash.String()
		}
		if image, err := i.runtime.ParseDigest(ref.Name()); err == nil {
			img = image.Name()
		}
		if img == "" {
			return digest, fmt.Errorf("unable to parse the reference '%s'", ref.Name())
		}
		if all {
			i.Index.Add(ctx, img, v.MediaType)
		}
	}

	return ref.Context().Digest(ref.Identifier()), err
}

func (i *ImageIndex) Remove(name string) (err error) {
	if ref, err := i.runtime.ParseDigest(name); err == nil {
		err = i.Index.Remove(ref)
		if err != nil {
			return err
		}
		return nil
	}
	if ref, err := i.runtime.ParseReference(name); err == nil {
		for _, v := range i.Instances[ref.Identifier()] {
			name := strings.Split(ref.Name(), ":")
			if ref, err := i.runtime.ParseDigest(name[0] + "@" + v.Hash.String()); err == nil {
				if err := i.Index.Remove(ref); err == nil {
					fmt.Printf("Successfully removed Image '%s'", ref.Name())
				}
			}
			err = nil
		}
	}
	return
}

func (i ImageIndex) Delete(name string) error {
	if _, err := i.runtime.ParseReference(name); err != nil {
		return err
	}

	return i.Index.Delete(name)
}
