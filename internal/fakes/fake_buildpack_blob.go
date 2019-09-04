package fakes

import (
	"bytes"
	"io"
	"io/ioutil"
	"os"

	"github.com/BurntSushi/toml"

	"github.com/buildpack/pack/builder"
	"github.com/buildpack/pack/internal/archive"
)

type fakeBuildpackBlob struct {
	tmpDir     string
	descriptor builder.BuildpackDescriptor
}

func NewFakeBuildpackBlob(tmpDir string, descriptor builder.BuildpackDescriptor) *fakeBuildpackBlob {
	return &fakeBuildpackBlob{
		tmpDir:     tmpDir,
		descriptor: descriptor,
	}
}

func (b fakeBuildpackBlob) Open() (io.ReadCloser, error) {
	return CreateBuildpackTGZ(b.tmpDir, b.descriptor)
}

func CreateBuildpackTGZ(tmpDir string, descriptor builder.BuildpackDescriptor) (*os.File, error) {
	bpTmpFile, err := ioutil.TempFile(tmpDir, "bp-*.tgz")
	if err != nil {
		return nil, err
	}

	buf := &bytes.Buffer{}
	if err = toml.NewEncoder(buf).Encode(descriptor); err != nil {
		return nil, err
	}

	if err = archive.CreateSingleFileTar(bpTmpFile.Name(), "buildpack.toml", buf.String()); err != nil {
		return nil, err
	}

	return bpTmpFile, nil
}
