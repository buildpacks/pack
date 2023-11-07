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

	"github.com/buildpacks/pack/internal/config"
)

type runtime struct {
	manifestListPath string
}

func NewRuntime() (runtime, error) {
	if value, ok := os.LookupEnv("XDG_RUNTIME_DIR"); ok {
		return runtime{
			manifestListPath: filepath.Join(value, "manifestList"),
		}, nil
	}
	if home, err := config.PackHome(); err == nil {
		return runtime{
			manifestListPath: filepath.Join(home, "manifestList"),
		}, nil
	}
	return runtime{}, fmt.Errorf("unable to runtime path")
}

func (r runtime) LookupImageIndex(name string) (index ImageIndex, err error) {
	n, err := r.ParseReference(name)
	if err != nil {
		return index, err
	}
	filepath := filepath.Join(r.manifestListPath, makeFilesafeName(n.Name()))
	_, err = os.Stat(filepath)
	if err != nil {
		return
	}
	manifestBytes, _ := os.ReadFile(filepath)
	if err != nil {
		return
	}
	var dockerManifest imgutil.DockerManifestList
	var ociImageIndex v1.ImageIndex
	if err := json.Unmarshal(manifestBytes, &dockerManifest); err != nil {
		return ImageIndex{}, err
	}
	if err := json.Unmarshal(manifestBytes, &ociImageIndex); err != nil {
		return ImageIndex{}, err
	}
	return ImageIndex{
		docker: dockerManifest,
		oci:    ociImageIndex,
	}, err
}

func (r runtime) ImageType(format string) (manifestType imgutil.MediaTypes) {
	switch format {
	case imgutil.ImageIndexTypes.OCI:
		return imgutil.ImageIndexTypes.OCI
	case imgutil.ImageIndexTypes.Index:
		return imgutil.ImageIndexTypes.Index
	default:
		return imgutil.ImageIndexTypes.Docker
	}
}

func (r runtime) ParseReference(image string) (ref name.Reference, err error) {
	return name.ParseReference(image)
}

func (r runtime) ParseDigest(image string) (ref name.Digest, err error) {
	return name.NewDigest(image)
}

func (r runtime) RemoveManifests(ctx context.Context, names []string) (err error) {
	for _, name := range names {
		name = makeFilesafeName(name)
		if _, err := os.Stat(filepath.Join(r.manifestListPath, name, name)); err == nil {
			err := os.Remove(filepath.Join(r.manifestListPath, name))
			if err != nil {
				errors = append(errors, err)
			}
		}
	}
	return
}

func makeFilesafeName(ref string) string {
	fileName := strings.Replace(ref, ":", "-", -1)
	return strings.Replace(fileName, "/", "_", -1)
}
