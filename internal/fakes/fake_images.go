package fakes

import (
	"encoding/json"
	"testing"

	"github.com/buildpack/imgutil/fakes"

	"github.com/buildpack/pack/builder"
	h "github.com/buildpack/pack/testhelpers"
)

func NewFakeBuilderImage(t *testing.T, name string, stackId, uid, gid string, metadata builder.Metadata) *fakes.Image {
	fakeBuilderImage := fakes.NewImage(name, "", "")
	h.AssertNil(t, fakeBuilderImage.SetLabel("io.buildpacks.stack.id", stackId))
	h.AssertNil(t, fakeBuilderImage.SetEnv("CNB_USER_ID", uid))
	h.AssertNil(t, fakeBuilderImage.SetEnv("CNB_GROUP_ID", gid))
	label, err := json.Marshal(&metadata)
	h.AssertNil(t, err)
	h.AssertNil(t, fakeBuilderImage.SetLabel("io.buildpacks.builder.metadata", string(label)))
	return fakeBuilderImage
}
