package asset

import (
	"context"
	"fmt"

	"github.com/buildpacks/imgutil"

	pubcfg "github.com/buildpacks/pack/config"
)

// TODO: -Dan- make me private
// imageFetcher is an interface representing the ability to fetch local and images.
type imageFetcher interface {
	Fetch(ctx context.Context, name string, daemon bool, pullPolicy pubcfg.PullPolicy) (imgutil.Image, error)
}

type PackageImageFetcher struct {
	imageFetcher
}

func NewPackageImageFetcher(imageFetcher imageFetcher) PackageImageFetcher {
	return PackageImageFetcher{
		imageFetcher: imageFetcher,
	}
}

// TODO allow for smooth cancels via ctrl+c when downloading (need to add a context in)
func (af PackageImageFetcher) FetchImageAssets(ctx context.Context, pullPolicy pubcfg.PullPolicy, imageNames ...string) ([]imgutil.Image, error) {
	result := []imgutil.Image{}
	for _, imageName := range imageNames {
		img, err := af.imageFetcher.Fetch(ctx, imageName, true, pullPolicy)
		if err != nil {
			return result, fmt.Errorf("unable to fetch asset image: %q", err)
		}
		result = append(result, img)
	}
	return result, nil
}
