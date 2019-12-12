package fakes

import (
	"encoding/json"
	"testing"

	"github.com/buildpacks/imgutil/fakes"

	"github.com/buildpacks/pack/internal/builder"
	"github.com/buildpacks/pack/internal/dist"
	h "github.com/buildpacks/pack/testhelpers"
)

func NewFakeBuilderImage(t *testing.T, name string, stackID, uid, gid string, metadata builder.Metadata, bpLayers dist.BuildpackLayers) *fakes.Image {
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

	return fakeBuilderImage
}
