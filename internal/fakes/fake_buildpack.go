package fakes

import (
	"bytes"
	"fmt"
	"io"

	"github.com/BurntSushi/toml"

	"github.com/buildpacks/pack/internal/archive"
	"github.com/buildpacks/pack/internal/dist"
)

type fakeBuildpack struct {
	descriptor dist.BuildpackDescriptor
	chmod      int64
}

// NewFakeBuildpack creates a fake buildpacks with contents:
//
// 	\_ /cnbs/buildpacks/{ID}
// 	\_ /cnbs/buildpacks/{ID}/{version}
// 	\_ /cnbs/buildpacks/{ID}/{version}/buildpack.toml
// 	\_ /cnbs/buildpacks/{ID}/{version}/bin
// 	\_ /cnbs/buildpacks/{ID}/{version}/bin/build
//  	build-contents
// 	\_ /cnbs/buildpacks/{ID}/{version}/bin/detect
//  	detect-contents
func NewFakeBuildpack(descriptor dist.BuildpackDescriptor, chmod int64) (dist.Buildpack, error) {
	return &fakeBuildpack{
		descriptor: descriptor,
		chmod:      chmod,
	}, nil
}

func (b *fakeBuildpack) Descriptor() dist.BuildpackDescriptor {
	return b.descriptor
}

func (b *fakeBuildpack) Open() (io.ReadCloser, error) {
	buf := &bytes.Buffer{}
	if err := toml.NewEncoder(buf).Encode(b.descriptor); err != nil {
		return nil, err
	}

	tarBuilder := archive.TarBuilder{}
	ts := archive.NormalizedDateTime
	tarBuilder.AddDir(fmt.Sprintf("/cnb/buildpacks/%s", b.descriptor.EscapedID()), b.chmod, ts)
	bpDir := fmt.Sprintf("/cnb/buildpacks/%s/%s", b.descriptor.EscapedID(), b.descriptor.Info.Version)
	tarBuilder.AddDir(bpDir, b.chmod, ts)
	tarBuilder.AddFile(bpDir+"/buildpack.toml", b.chmod, ts, buf.Bytes())

	if len(b.descriptor.Order) == 0 {
		tarBuilder.AddDir(bpDir+"/bin", b.chmod, ts)
		tarBuilder.AddFile(bpDir+"/bin/build", b.chmod, ts, []byte("build-contents"))
		tarBuilder.AddFile(bpDir+"/bin/detect", b.chmod, ts, []byte("detect-contents"))
	}

	return tarBuilder.Reader(archive.DefaultTarWriterFactory), nil
}
