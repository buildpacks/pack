package fakes

import (
	"io"
	"io/ioutil"
	"testing"

	"github.com/buildpacks/pack/internal/dist"
	h "github.com/buildpacks/pack/testhelpers"
)

func CreateBuildpackTar(t *testing.T, tmpDir string, descriptor dist.BuildpackDescriptor) string {
	buildpack, err := NewFakeBuildpackBlob(descriptor, 0777)
	h.AssertNil(t, err)

	tempFile, err := ioutil.TempFile(tmpDir, "bp-*.tar")
	h.AssertNil(t, err)
	defer tempFile.Close()

	reader, err := buildpack.Open()
	h.AssertNil(t, err)

	_, err = io.Copy(tempFile, reader)
	h.AssertNil(t, err)

	return tempFile.Name()
}
