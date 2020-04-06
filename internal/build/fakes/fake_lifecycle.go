package fakes

import (
	"bytes"

	"github.com/apex/log"
	"github.com/docker/docker/client"

	"github.com/buildpacks/pack/internal/build"
	ilogging "github.com/buildpacks/pack/internal/logging"
)

func NewFakeLifecycle(logVerbose bool, ops ...func(*build.LifecycleOptions)) (*build.Lifecycle, error) {
	docker, err := client.NewClientWithOpts(client.FromEnv, client.WithVersion("1.38"))
	if err != nil {
		return nil, err
	}

	var outBuf bytes.Buffer
	logger := ilogging.NewLogWithWriters(&outBuf, &outBuf)
	if logVerbose {
		logger.Level = log.DebugLevel
	}

	lifecycle := build.NewLifecycle(docker, logger)

	defaultBuilder, err := NewFakeBuilder()
	if err != nil {
		return nil, err
	}

	opts := build.LifecycleOptions{
		AppPath:    "some-app-path",
		Builder:    defaultBuilder,
		HTTPProxy:  "some-http-proxy",
		HTTPSProxy: "some-https-proxy",
		NoProxy:    "some-no-proxy",
	}

	for _, op := range ops {
		op(&opts)
	}

	lifecycle.Setup(opts)
	return lifecycle, nil
}

func WithBuilder(builder *FakeBuilder) func(*build.LifecycleOptions) {
	return func(opts *build.LifecycleOptions) {
		opts.Builder = builder
	}
}
