package build

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
	"github.com/buildpacks/imgutil/layout/sparse"
	"github.com/buildpacks/lifecycle/buildpack"
	"github.com/buildpacks/lifecycle/cmd"
	"github.com/buildpacks/pack/pkg/logging"
	"github.com/google/go-containerregistry/pkg/name"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/daemon"
	"golang.org/x/sync/errgroup"
)

const (
	DockerfileKindBuild = "build"
	DockerfileKindRun   = "run"
)

type Extensions struct {
	Extensions []buildpack.GroupElement
}

func (extensions *Extensions) DockerFiles(kind string, path string, logger logging.Logger) ([]buildpack.DockerfileInfo, error) {
	var dockerfiles []buildpack.DockerfileInfo
	for _, ext := range extensions.Extensions {
		dockerfile, err := extensions.ReadDockerFile(path, kind, ext.ID)
		if err != nil {
			return nil, err
		}
		if dockerfile != nil {
			logger.Debugf("Found %s Dockerfile for extension '%s'", kind, ext.ID)
			switch kind {
			case DockerfileKindBuild:
				// will implement later
			case DockerfileKindRun:
				buildpack.ValidateRunDockerfile(dockerfile, logger)
			default:
				return nil, fmt.Errorf("unknown dockerfile kind: %s", kind)
			}
			dockerfiles = append(dockerfiles, *dockerfile)
		}
	}
	return dockerfiles, nil
}

func (extensions *Extensions) ReadDockerFile(path string, kind string, extID string) (*buildpack.DockerfileInfo, error) {
	dockerfilePath := filepath.Join(path, kind, escapeID(extID), "Dockerfile")
	if _, err := os.Stat(dockerfilePath); err != nil {
		return nil, nil
	}
	return &buildpack.DockerfileInfo{
		ExtensionID: extID,
		Kind:        kind,
		Path:        dockerfilePath,
	}, nil
}

func (extensions *Extensions) SetExtensions(path string, logger logging.Logger) error {
	groupExt, err := readExtensionsGroup(path)
	if err != nil {
		return fmt.Errorf("reading group: %w", err)
	}
	for i := range groupExt {
		groupExt[i].Extension = true
	}
	for _, groupEl := range groupExt {
		if err = cmd.VerifyBuildpackAPI(groupEl.Kind(), groupEl.String(), groupEl.API, logger); err != nil {
			return err
		}
	}
	extensions.Extensions = groupExt
	fmt.Println("extensions.Extensions", extensions.Extensions)
	return nil
}

func readExtensionsGroup(path string) ([]buildpack.GroupElement, error) {
	var group buildpack.Group
	_, err := toml.DecodeFile(filepath.Join(path, "group.toml"), &group)
	for e := range group.GroupExtensions {
		group.GroupExtensions[e].Extension = true
		group.GroupExtensions[e].Optional = true
	}
	return group.GroupExtensions, err
}

func escapeID(id string) string {
	return strings.ReplaceAll(id, "/", "_")
}

func SaveLayers(group *errgroup.Group, image v1.Image, origTopLayerHash string, dest string) error {
	layoutPath, err := sparse.NewImage(dest, image)
	if err != nil {
		fmt.Println("sparse.NewImage err", err)
		return err
	}
	if err = layoutPath.Save(); err != nil {
		return err
	}
	if err != nil {
		fmt.Println("sparse.NewImage err", err)
		return err
	}
	layers, err := image.Layers()
	if err != nil {
		return fmt.Errorf("getting image layers: %w", err)
	}
	var (
		currentHash  v1.Hash
		needsCopying bool
	)
	if origTopLayerHash == "" {
		needsCopying = true
	}
	for _, currentLayer := range layers {
		currentHash, err = currentLayer.Digest()
		if err != nil {
			return fmt.Errorf("getting layer hash: %w", err)
		}
		switch {
		case needsCopying:
			currentLayer := currentLayer
			group.Go(func() error {
				return copyLayer(currentLayer, dest)
			})
		case currentHash.String() == origTopLayerHash:
			needsCopying = true
			continue
		default:
			continue
		}
	}
	return nil
}

func copyLayer(layer v1.Layer, toSparseImage string) error {
	digest, err := layer.Digest()
	if err != nil {
		return err
	}
	f, err := os.Create(filepath.Join(toSparseImage, "blobs", digest.Algorithm, digest.Hex))
	if err != nil {
		return err
	}
	defer f.Close()
	rc, err := layer.Compressed()
	if err != nil {
		return err
	}
	defer rc.Close()
	_, err = io.Copy(f, rc)
	return err
}

func topLayerHash(image *string) (string, error) {
	baseRef, err := name.ParseReference(*image)
	if err != nil {
		return "", fmt.Errorf("failed to parse reference: %v", err)
	}
	baseImage, err := daemon.Image(baseRef)
	if err != nil {
		return "", fmt.Errorf("failed to get v1.Image: %v", err)
	}
	baseManifest, err := baseImage.Manifest()
	if err != nil {
		return "", fmt.Errorf("getting image manifest: %w", err)
	}
	baseLayers := baseManifest.Layers
	topLayerHash := baseLayers[len(baseLayers)-1].Digest.String()
	return topLayerHash, nil
}
