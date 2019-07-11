package mocks

import (
	"context"

	"github.com/buildpack/pack/build"
)

type FakeLifecycle struct {
	Opts build.LifecycleOptions
}

func (f *FakeLifecycle) Execute(ctx context.Context, opts build.LifecycleOptions) error {
	f.Opts = opts
	return nil
}
