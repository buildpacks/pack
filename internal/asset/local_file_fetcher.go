package asset

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/pkg/errors"

	blob2 "github.com/buildpacks/pack/internal/blob"
	"github.com/buildpacks/pack/internal/oci"
)

// PackageFileFetcher is an implementation of FileFetcher, it provides methods to
// fetch OCI Layout files from the filesystem
type PackageFileFetcher struct{}

// NewPackageFileFetcher is a constructor and should be used to create new instances of
// PackageFileFetcher
func NewPackageFileFetcher() PackageFileFetcher {
	return PackageFileFetcher{}
}

// FetchFileAssets talks a list of paths and a working directory,
// each path may be local or absolute, it the path is local it
// is resolved to a absolute path using workingDir.
// we then attemp to read each path as an OCI LayoutPackage and return it.
func (af PackageFileFetcher) FetchFileAssets(ctx context.Context, workingDir string, fileAssets ...string) ([]*oci.LayoutPackage, error) {
	result := []*oci.LayoutPackage{}
	for _, assetFile := range fileAssets {
		assetPath, ok := localFile(assetFile, workingDir)
		switch {
		case ok:
			p, err := oci.NewLayoutPackage(blob2.NewBlob(assetPath, blob2.RawOption))
			if err != nil {
				return []*oci.LayoutPackage{}, errors.Wrap(err, "unable to read asset as OCI blob")
			}
			result = append(result, p)
		default:
			return []*oci.LayoutPackage{}, fmt.Errorf("unable to fetch file asset %q", assetFile)
		}
	}

	return result, nil
}

func localFile(path, relativeBaseDir string) (string, bool) {
	if !filepath.IsAbs(path) {
		path = filepath.Join(relativeBaseDir, path)
	}

	if _, err := os.Stat(path); err == nil {
		return path, true
	}

	return "", false
}
