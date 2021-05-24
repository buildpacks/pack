package image

import (
	"context"
	"github.com/buildpacks/imgutil"
	"github.com/buildpacks/pack/config"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/pkg/errors"
	"path"
	"strings"
)

type ImageFetcher interface {
	Fetch(ctx context.Context, name string, daemon bool, pullPolicy config.PullPolicy) (imgutil.Image, error)
}

type ProxiedFetcher struct {
	proxyHost string
	fetcher   ImageFetcher
}

func (pw ProxiedFetcher) Fetch(ctx context.Context, imgName string, daemon bool, pullPolicy config.PullPolicy) (imgutil.Image, error) {
	proxiedImage, err := ProxyImage(pw.proxyHost, imgName)
	if err != nil {
		return nil, errors.Wrap(err, "unable to create proxy image name")
	}
	return pw.fetcher.Fetch(ctx, proxiedImage.Name(), daemon, pullPolicy)
}

func NewProxiedFetcher(proxyHost string, fetcher ImageFetcher) ImageFetcher {
	return ProxiedFetcher{
		proxyHost: strings.TrimRight(proxyHost,"/"),
		fetcher:   fetcher,
	}
}

func ProxyImage(proxyHost, imageName string) (name.Reference, error) {
	parsedRef, err := name.ParseReference(imageName, name.WeakValidation)
	if err != nil {
		return nil, errors.Wrap(err, "image name is invalid")
	}
	reg := parsedRef.Context().RegistryStr()
	if strings.Contains(proxyHost, reg) {
		return parsedRef, nil
	}
	proxiedRef, err := name.ParseReference(path.Join(proxyHost, strings.TrimPrefix(parsedRef.Name(), reg)), name.WeakValidation)
	if err != nil {
		return nil, errors.Wrap(err, "proxied image name is invalid")
	}
	return proxiedRef, err
}


