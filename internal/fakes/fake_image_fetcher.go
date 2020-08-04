package fakes

import (
	"context"

	"github.com/buildpacks/pack/config"

	"github.com/buildpacks/imgutil"
	"github.com/pkg/errors"

	"github.com/buildpacks/pack/internal/image"
)

type FetchArgs struct {
	Daemon     bool
	Pull       bool //TODO: REMOVE
	PullPolicy config.PullPolicy
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

func (f *FakeImageFetcher) Fetch(ctx context.Context, name string, daemon bool, policy config.PullPolicy) (imgutil.Image, error) {
	f.FetchCalls[name] = &FetchArgs{Daemon: daemon, Pull: boolFromPolicy(policy), PullPolicy: policy}

	ri, remoteFound := f.RemoteImages[name]

	if daemon {
		if remoteFound && policy == config.PullAlways {
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

func boolFromPolicy(policy config.PullPolicy) bool {
	return policy == config.PullAlways
}
