package fakes

import (
	"context"

	"github.com/buildpacks/pack/config"
	"github.com/buildpacks/pack/image"
	"github.com/buildpacks/pack/internal/builder"
)

type FakeInspectableFetcher struct {
	InspectableToReturn *FakeInspectable
	ErrorToReturn       error

	CallCount int

	ReceivedName       string
	ReceivedDaemon     bool
	ReceivedPullPolicy config.PullPolicy
}

func (f *FakeInspectableFetcher) Fetch(ctx context.Context, name string, options image.FetchOptions) (builder.Inspectable, error) {
	f.CallCount++

	f.ReceivedName = name
	f.ReceivedDaemon = options.Daemon
	f.ReceivedPullPolicy = options.PullPolicy

	return f.InspectableToReturn, f.ErrorToReturn
}
