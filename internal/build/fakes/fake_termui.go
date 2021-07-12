package fakes

import (
	"github.com/buildpacks/pack/internal/build"
	"github.com/buildpacks/pack/internal/container"
)

type FakeTermui struct {
	handler container.Handler
}

func NewFakeTermui(handler container.Handler) *FakeTermui {
	return &FakeTermui{
		handler: handler,
	}
}

func (f *FakeTermui) Run(funk func()) error {
	return nil
}

func (f *FakeTermui) Handler() container.Handler {
	return f.handler
}

func WithTermui(screen build.Termui) func(*build.LifecycleOptions) {
	return func(opts *build.LifecycleOptions) {
		opts.Interactive = true
		opts.Termui = screen
	}
}
