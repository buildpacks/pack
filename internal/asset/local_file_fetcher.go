package asset

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	blob2 "github.com/buildpacks/pack/internal/blob"
	"github.com/buildpacks/pack/internal/ocipackage"
)

type LocalFileFetcher struct{}

func NewLocalFileFetcher() LocalFileFetcher {
	return LocalFileFetcher{}
}

func (af LocalFileFetcher) FetchFileAssets(ctx context.Context, workingDir string, fileAssets ...string) ([]*ocipackage.OciLayoutPackage, error) {
	result := []*ocipackage.OciLayoutPackage{}
	for _, assetFile := range fileAssets {
		assetPath, ok := localFile(assetFile, workingDir)
		switch {
		case ok:
			p, err := ocipackage.NewOCILayoutPackage(blob2.NewBlob(assetPath, blob2.RawOption))
			if err != nil {
				// TODO -Dan- handle error
				panic(err)
			}
			result = append(result, p)
		default:
			return []*ocipackage.OciLayoutPackage{}, fmt.Errorf("unable to fetch file asset %q", assetFile)
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
