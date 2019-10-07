package fakes

import (
	"bytes"
	"io"
	"time"

	"github.com/BurntSushi/toml"

	"github.com/buildpack/pack/dist"
	"github.com/buildpack/pack/internal/archive"
)

type fakeBuildpackBlob struct {
	descriptor dist.BuildpackDescriptor
	chmod      int64
}

// NewBuildpackFromDescriptor creates a fake buildpacks for testing purposes where tar contents are such:
//
// 	\_ buildpack.toml
// 	\_ bin
// 	\_ bin/build
//  	build-contents
// 	\_ bin/detect
//  	detect-contents
//
func NewBuildpackFromDescriptor(descriptor dist.BuildpackDescriptor, chmod int64) (dist.Buildpack, error) {
	return &fakeBuildpackBlob{
		descriptor: descriptor,
		chmod:      chmod,
	}, nil
}

func (b *fakeBuildpackBlob) Descriptor() dist.BuildpackDescriptor {
	return b.descriptor
}

func (b *fakeBuildpackBlob) Open() (reader io.ReadCloser, err error) {
	buf := &bytes.Buffer{}
	if err = toml.NewEncoder(buf).Encode(b.descriptor); err != nil {
		return nil, err
	}

	tarBuilder := archive.TarBuilder{}
	tarBuilder.AddFile("buildpack.toml", b.chmod, time.Now(), buf.Bytes())
	tarBuilder.AddDir("bin", b.chmod, time.Now())
	tarBuilder.AddFile("bin/build", b.chmod, time.Now(), []byte("build-contents"))
	tarBuilder.AddFile("bin/detect", b.chmod, time.Now(), []byte("detect-contents"))

	return tarBuilder.Reader(), err
}
