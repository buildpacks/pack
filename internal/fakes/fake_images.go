package fakes

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/buildpacks/imgutil/fakes"

	"github.com/buildpacks/pack/internal/builder"
	"github.com/buildpacks/pack/internal/dist"
	h "github.com/buildpacks/pack/testhelpers"
)

func NewFakeBuilderImage(t *testing.T, tmpDir, name string, stackID, uid, gid string, metadata builder.Metadata, bpLayers dist.BuildpackLayers) *fakes.Image {
	fakeBuilderImage := fakes.NewImage(name, "", nil)

	h.AssertNil(t, fakeBuilderImage.SetLabel("io.buildpacks.stack.id", stackID))
	h.AssertNil(t, fakeBuilderImage.SetEnv("CNB_USER_ID", uid))
	h.AssertNil(t, fakeBuilderImage.SetEnv("CNB_GROUP_ID", gid))

	label, err := json.Marshal(&metadata)
	h.AssertNil(t, err)
	h.AssertNil(t, fakeBuilderImage.SetLabel("io.buildpacks.builder.metadata", string(label)))

	label, err = json.Marshal(&bpLayers)
	h.AssertNil(t, err)
	h.AssertNil(t, fakeBuilderImage.SetLabel("io.buildpacks.buildpack.layers", string(label)))

	for bpID, v := range bpLayers {
		for bpVersion, bpLayerInfo := range v {
			buildpack, err := NewFakeBuildpack(dist.BuildpackDescriptor{
				API: bpLayerInfo.API,
				Info: dist.BuildpackInfo{
					ID:      bpID,
					Version: bpVersion,
				},
				Stacks: bpLayerInfo.Stacks,
				Order:  bpLayerInfo.Order,
			}, 0755)
			h.AssertNil(t, err)

			buildpackTar := createBuildpackTar(t, tmpDir, buildpack)
			err = fakeBuilderImage.AddLayer(buildpackTar)
			h.AssertNil(t, err)
		}
	}

	return fakeBuilderImage
}

func createBuildpackTar(t *testing.T, tmpDir string, buildpack dist.Buildpack) string {
	f, err := os.Create(filepath.Join(tmpDir, fmt.Sprintf(
		"%s.%s.tar",
		buildpack.Descriptor().Info.ID,
		buildpack.Descriptor().Info.Version),
	))
	h.AssertNil(t, err)
	defer f.Close()

	reader, err := buildpack.Open()
	h.AssertNil(t, err)

	_, err = io.Copy(f, reader)
	h.AssertNil(t, err)

	return f.Name()
}
