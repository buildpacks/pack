package mocks

import (
	"encoding/json"
	"testing"

	"github.com/buildpack/imgutil/fakes"

	"github.com/buildpack/pack/builder"
	"github.com/buildpack/pack/stack"
	h "github.com/buildpack/pack/testhelpers"
)

func NewFakeBuilderImage(t *testing.T, name string, buildpacks []builder.BuildpackMetadata, config builder.Config) *fakes.Image {
	fakeBuilderImage := fakes.NewImage(name, "", "")
	h.AssertNil(t, fakeBuilderImage.SetLabel("io.buildpacks.stack.id", config.Stack.ID))
	h.AssertNil(t, fakeBuilderImage.SetEnv("CNB_USER_ID", "1234"))
	h.AssertNil(t, fakeBuilderImage.SetEnv("CNB_GROUP_ID", "4321"))
	metadata := builder.Metadata{
		Buildpacks: buildpacks,
		Groups:     config.Groups,
		Stack: stack.Metadata{
			RunImage: stack.RunImageMetadata{
				Image:   config.Stack.RunImage,
				Mirrors: config.Stack.RunImageMirrors,
			},
		},
	}
	label, err := json.Marshal(&metadata)
	h.AssertNil(t, err)
	h.AssertNil(t, fakeBuilderImage.SetLabel("io.buildpacks.builder.metadata", string(label)))
	return fakeBuilderImage
}
