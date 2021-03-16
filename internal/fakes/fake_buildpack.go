package fakes

import (
	"bytes"
	"fmt"
	"io"
	"path/filepath"

	"github.com/BurntSushi/toml"

	"github.com/buildpacks/pack/internal/dist"
	"github.com/buildpacks/pack/pkg/archive"
)

type fakeBuildpack struct {
	descriptor dist.BuildpackDescriptor
	chmod      int64
	options    []FakeBuildpackOption
}

type fakeBuildpackConfig struct {
	// maping of extrafilename to stringified contents
	ExtraFiles map[string]string
}

func newFakeBuildpackConfig() *fakeBuildpackConfig {
	return &fakeBuildpackConfig{ExtraFiles: map[string]string{}}
}

type FakeBuildpackOption func(*fakeBuildpackConfig)

func WithExtraBuildpackContents(filename, contents string) FakeBuildpackOption {
	return func(f *fakeBuildpackConfig) {
		f.ExtraFiles[filename] = contents
	}
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
func NewFakeBuildpack(descriptor dist.BuildpackDescriptor, chmod int64, options ...FakeBuildpackOption) (dist.Buildpack, error) {
	return &fakeBuildpack{
		descriptor: descriptor,
		chmod:      chmod,
		options:    options,
	}, nil
}

func (b *fakeBuildpack) Descriptor() dist.BuildpackDescriptor {
	return b.descriptor
}

func (b *fakeBuildpack) Open() (io.ReadCloser, error) {
	fConfig := newFakeBuildpackConfig()
	for _, option := range b.options {
		option(fConfig)
	}

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

	for extraFilename, extraContents := range fConfig.ExtraFiles {
		tarBuilder.AddFile(filepath.Join(bpDir, extraFilename), b.chmod, ts, []byte(extraContents))
	}

	return tarBuilder.Reader(archive.DefaultTarWriterFactory()), nil
}
