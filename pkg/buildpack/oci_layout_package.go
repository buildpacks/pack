package buildpack

import (
	"archive/tar"
	"compress/gzip"
	"encoding/json"
	stderrors "errors"
	"fmt"
	"io"
	"path"
	"strings"

	v1 "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/pkg/errors"

	"github.com/buildpacks/pack/internal/ioutil"
	"github.com/buildpacks/pack/internal/paths"
	"github.com/buildpacks/pack/internal/style"
	"github.com/buildpacks/pack/pkg/archive"
	blob2 "github.com/buildpacks/pack/pkg/blob"
	"github.com/buildpacks/pack/pkg/dist"
)

// IsOCILayoutBlob checks whether a blob is in OCI layout format.
func IsOCILayoutBlob(blob blob2.Blob) (bool, error) {
	readCloser, err := blob.Open()
	if err != nil {
		return false, err
	}
	defer readCloser.Close()

	_, _, err = archive.ReadTarEntry(readCloser, v1.ImageLayoutFile)
	if err != nil {
		if archive.IsEntryNotExist(err) {
			return false, nil
		}

		return false, err
	}

	return true, nil
}

// BuildpacksFromOCILayoutBlob constructs buildpacks from a blob in OCI layout format.
func BuildpacksFromOCILayoutBlob(blob Blob) (mainBP BuildModule, dependencies []BuildModule, err error) {
	layoutPackage, err := newOCILayoutPackage(blob, KindBuildpack, nil)
	if err != nil {
		return nil, nil, err
	}

	return extractBuildpacks(layoutPackage)
}

// ExtensionsFromOCILayoutBlob constructs extensions from a blob in OCI layout format.
func ExtensionsFromOCILayoutBlob(blob Blob) (mainExt BuildModule, err error) {
	layoutPackage, err := newOCILayoutPackage(blob, KindExtension, nil)
	if err != nil {
		return nil, err
	}

	return extractExtensions(layoutPackage)
}

func ConfigFromOCILayoutBlob(blob Blob) (config v1.ImageConfig, err error) {
	layoutPackage, err := newOCILayoutPackage(blob, KindBuildpack, nil)
	if err != nil {
		return v1.ImageConfig{}, err
	}
	return layoutPackage.imageInfo.Config, nil
}

type ociLayoutPackage struct {
	imageInfo v1.Image
	manifest  v1.Manifest
	blob      Blob
}

// BuildpacksFromOCILayoutBlobForTarget selects a platform before extracting its modules.
func BuildpacksFromOCILayoutBlobForTarget(blob Blob, target dist.Target) (BuildModule, []BuildModule, error) {
	pkg, err := newOCILayoutPackage(blob, KindBuildpack, &target)
	if err != nil {
		return nil, nil, err
	}
	return extractBuildpacks(pkg)
}

// ExtensionsFromOCILayoutBlobForTarget selects the requested extension platform.
func ExtensionsFromOCILayoutBlobForTarget(blob Blob, target dist.Target) (BuildModule, error) {
	pkg, err := newOCILayoutPackage(blob, KindExtension, &target)
	if err != nil {
		return nil, err
	}
	return extractExtensions(pkg)
}

func newOCILayoutPackage(blob Blob, kind string, target *dist.Target) (*ociLayoutPackage, error) {
	index := &v1.Index{}

	if err := unmarshalJSONFromBlob(blob, v1.ImageIndexFile, index); err != nil {
		return nil, err
	}

	manifest, imageInfo, err := findLayoutImage(blob, *index, target, 0)
	if stderrors.Is(err, errLayoutPlatformNotFound) {
		if target == nil {
			return nil, errors.New("unable to find manifest")
		}
		return nil, errors.Errorf("unable to find manifest for target %s", target.ValuesAsPlatform())
	}
	if err != nil {
		return nil, err
	}
	var layersLabel string
	switch kind {
	case KindBuildpack:
		layersLabel = imageInfo.Config.Labels[dist.BuildpackLayersLabel]
		if layersLabel == "" {
			return nil, errors.Errorf("label %s not found", style.Symbol(dist.BuildpackLayersLabel))
		}
	case KindExtension:
		layersLabel = imageInfo.Config.Labels[dist.ExtensionLayersLabel]
		if layersLabel == "" {
			return nil, errors.Errorf("label %s not found", style.Symbol(dist.ExtensionLayersLabel))
		}
	default:
		return nil, fmt.Errorf("unknown module kind: %s", kind)
	}

	bpLayers := dist.ModuleLayers{}
	if err := json.Unmarshal([]byte(layersLabel), &bpLayers); err != nil {
		return nil, errors.Wrap(err, "unmarshaling layers label")
	}

	return &ociLayoutPackage{
		imageInfo: *imageInfo,
		manifest:  *manifest,
		blob:      blob,
	}, nil
}

func (o *ociLayoutPackage) Label(name string) (value string, err error) {
	return o.imageInfo.Config.Labels[name], nil
}

func (o *ociLayoutPackage) GetLayer(diffID string) (io.ReadCloser, error) {
	index := -1
	for i, dID := range o.imageInfo.RootFS.DiffIDs {
		if dID.String() == diffID {
			index = i
			break
		}
	}
	if index == -1 {
		return nil, errors.Errorf("layer %s not found in rootfs", style.Symbol(diffID))
	}

	layerDescriptor := o.manifest.Layers[index]
	layerPath := paths.CanonicalTarPath(pathFromDescriptor(layerDescriptor))

	blobReader, err := o.blob.Open()
	if err != nil {
		return nil, err
	}

	tr := tar.NewReader(blobReader)
	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, errors.Wrap(err, "failed to get next tar entry")
		}

		if paths.CanonicalTarPath(header.Name) == layerPath {
			finalReader := blobReader

			if strings.HasSuffix(layerDescriptor.MediaType, "gzip") {
				finalReader, err = gzip.NewReader(tr)
				if err != nil {
					return nil, err
				}
			}

			return ioutil.NewReadCloserWrapper(finalReader, func() error {
				if err := finalReader.Close(); err != nil {
					return err
				}

				return blobReader.Close()
			}), nil
		}
	}

	if err := blobReader.Close(); err != nil {
		return nil, err
	}

	return nil, errors.Errorf("layer blob %s not found", style.Symbol(layerPath))
}

func pathFromDescriptor(descriptor v1.Descriptor) string {
	return path.Join("/blobs", descriptor.Digest.Algorithm().String(), descriptor.Digest.Encoded())
}

func unmarshalJSONFromBlob(blob Blob, path string, obj interface{}) error {
	reader, err := blob.Open()
	if err != nil {
		return err
	}
	defer reader.Close()

	_, contents, err := archive.ReadTarEntry(reader, path)
	if err != nil {
		return err
	}

	if err = json.Unmarshal(contents, obj); err != nil {
		return err
	}

	return nil
}
