package fakes

import (
	"bytes"
	"io"
	"time"

	"github.com/BurntSushi/toml"
	iarchive "github.com/buildpacks/imgutil/archive"

	"github.com/buildpacks/pack/internal/archive"
	"github.com/buildpacks/pack/internal/dist"
)

type fakeBuildpackBlob struct {
	descriptor dist.BuildpackDescriptor
	chmod      int64
}

// NewFakeBuildpackBlob creates a fake blob with contents:
//
// 	\_ buildpack.toml
// 	\_ bin
// 	\_ bin/build
//  	build-contents
// 	\_ bin/detect
//  	detect-contents
func NewFakeBuildpackBlob(descriptor dist.BuildpackDescriptor, chmod int64) (dist.Blob, error) {
	return &fakeBuildpackBlob{
		descriptor: descriptor,
		chmod:      chmod,
	}, nil
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

	return tarBuilder.Reader(iarchive.DefaultTarWriterFactory), err
}
