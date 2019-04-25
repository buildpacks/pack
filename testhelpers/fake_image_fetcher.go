package testhelpers

import (
	"context"

	"github.com/buildpack/imgutil"
	"github.com/pkg/errors"

	"github.com/buildpack/pack/image"
)

type FetchArgs struct {
	Daemon bool
	Pull   bool
}

type FakeImageFetcher struct {
	LocalImages  map[string]imgutil.Image
	RemoteImages map[string]imgutil.Image
	FetchCalls   map[string]*FetchArgs
}

func NewFakeImageFetcher() *FakeImageFetcher {
	return &FakeImageFetcher{
		LocalImages:  map[string]imgutil.Image{},
		RemoteImages: map[string]imgutil.Image{},
		FetchCalls:   map[string]*FetchArgs{},
	}
}

func (f *FakeImageFetcher) Fetch(ctx context.Context, name string, daemon, pull bool) (imgutil.Image, error) {
	f.FetchCalls[name] = &FetchArgs{Daemon: daemon, Pull: pull}

	ri, remoteFound := f.RemoteImages[name]

	if daemon {
		if remoteFound && pull {
			f.LocalImages[name] = ri
		}
		li, localFound := f.LocalImages[name]
		if !localFound {
			return nil, errors.Wrapf(image.ErrNotFound, "image '%s' does not exist on the daemon", name)
		}
		return li, nil
	}

	if !remoteFound {
		return nil, errors.Wrapf(image.ErrNotFound, "image '%s' does not exist in registry", name)
	}

	return ri, nil
}
