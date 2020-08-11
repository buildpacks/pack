package fakes

import (
	"github.com/Masterminds/semver"
	"github.com/buildpacks/lifecycle/api"

	"github.com/buildpacks/pack/internal/build"
	"github.com/buildpacks/pack/internal/builder"
)

type FakeBuilder struct {
	ReturnForName                string
	ReturnForUID                 int
	ReturnForGID                 int
	ReturnForLifecycleDescriptor builder.LifecycleDescriptor
	ReturnForStack               builder.StackMetadata
}

func NewFakeBuilder(ops ...func(*FakeBuilder)) (*FakeBuilder, error) {
	infoVersion, err := semver.NewVersion("12.34")
	if err != nil {
		return nil, err
	}

	platformAPIVersion, err := api.NewVersion("23.45")
	if err != nil {
		return nil, err
	}

	buildpackVersion, err := api.NewVersion("34.56")
	if err != nil {
		return nil, err
	}

	fakeBuilder := &FakeBuilder{
		ReturnForName: "some-builder-name",
		ReturnForUID:  99,
		ReturnForGID:  99,
		ReturnForLifecycleDescriptor: builder.LifecycleDescriptor{
			APIs: builder.LifecycleAPIs{
				Buildpack: builder.APIVersions{
					Supported: builder.APISet{buildpackVersion},
				},
				Platform: builder.APIVersions{
					Supported: builder.APISet{platformAPIVersion},
				},
			},
			Info: builder.LifecycleInfo{
				Version: &builder.Version{Version: *infoVersion},
			},
		},
		ReturnForStack: builder.StackMetadata{},
	}

	for _, op := range ops {
		op(fakeBuilder)
	}

	return fakeBuilder, nil
}

func WithName(name string) func(*FakeBuilder) {
	return func(builder *FakeBuilder) {
		builder.ReturnForName = name
	}
}

func WithUID(uid int) func(*FakeBuilder) {
	return func(builder *FakeBuilder) {
		builder.ReturnForUID = uid
	}
}

func WithGID(gid int) func(*FakeBuilder) {
	return func(builder *FakeBuilder) {
		builder.ReturnForGID = gid
	}
}

func (b *FakeBuilder) Name() string {
	return b.ReturnForName
}

func (b *FakeBuilder) UID() int {
	return b.ReturnForUID
}

func (b *FakeBuilder) GID() int {
	return b.ReturnForGID
}

func (b *FakeBuilder) LifecycleDescriptor() builder.LifecycleDescriptor {
	return b.ReturnForLifecycleDescriptor
}

func (b *FakeBuilder) Stack() builder.StackMetadata {
	return b.ReturnForStack
}

func WithBuilder(builder *FakeBuilder) func(*build.LifecycleOptions) {
	return func(opts *build.LifecycleOptions) {
		opts.Builder = builder
	}
}
