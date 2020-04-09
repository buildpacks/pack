package fakes

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/BurntSushi/toml"
	"github.com/buildpacks/imgutil/fakes"

	"github.com/buildpacks/pack/internal/archive"
	"github.com/buildpacks/pack/internal/builder"
	"github.com/buildpacks/pack/internal/dist"
	h "github.com/buildpacks/pack/testhelpers"
)

func NewFakeBuilderImage(t *testing.T, tmpDir, name string, stackID, uid, gid string, metadata builder.Metadata, bpLayers dist.BuildpackLayers, order dist.Order) *fakes.Image {
	fakeBuilderImage := fakes.NewImage(name, "", nil)

	h.AssertNil(t, fakeBuilderImage.SetLabel("io.buildpacks.stack.id", stackID))
	h.AssertNil(t, fakeBuilderImage.SetEnv("CNB_USER_ID", uid))
	h.AssertNil(t, fakeBuilderImage.SetEnv("CNB_GROUP_ID", gid))

	h.AssertNil(t, dist.SetLabel(fakeBuilderImage, "io.buildpacks.builder.metadata", metadata))
	h.AssertNil(t, dist.SetLabel(fakeBuilderImage, "io.buildpacks.buildpack.layers", bpLayers))

	for bpID, v := range bpLayers {
		for bpVersion, bpLayerInfo := range v {
			bpInfo := dist.BuildpackInfo{
				ID:      bpID,
				Version: bpVersion,
			}

			buildpack, err := NewFakeBuildpack(dist.BuildpackDescriptor{
				API:    bpLayerInfo.API,
				Info:   bpInfo,
				Stacks: bpLayerInfo.Stacks,
				Order:  bpLayerInfo.Order,
			}, 0755)
			h.AssertNil(t, err)

			buildpackTar := createBuildpackTar(t, tmpDir, buildpack)
			err = fakeBuilderImage.AddLayer(buildpackTar)
			h.AssertNil(t, err)
		}
	}

	h.AssertNil(t, dist.SetLabel(fakeBuilderImage, "io.buildpacks.buildpack.order", order))

	tarBuilder := archive.TarBuilder{}
	orderTomlBytes := &bytes.Buffer{}
	h.AssertNil(t, toml.NewEncoder(orderTomlBytes).Encode(orderTOML{Order: order}))
	tarBuilder.AddFile("/cnb/order.toml", 0777, archive.NormalizedDateTime, orderTomlBytes.Bytes())

	orderTar := filepath.Join(tmpDir, fmt.Sprintf("order.%s.toml", h.RandString(8)))
	h.AssertNil(t, tarBuilder.WriteToPath(orderTar, archive.DefaultTarWriterFactory))
	h.AssertNil(t, fakeBuilderImage.AddLayer(orderTar))

	return fakeBuilderImage
}

type orderTOML struct {
	Order dist.Order `toml:"order"`
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
