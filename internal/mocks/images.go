package mocks

import (
	"encoding/json"
	"testing"

	"github.com/Masterminds/semver"
	"github.com/buildpack/imgutil/fakes"

	"github.com/buildpack/pack/builder"
	"github.com/buildpack/pack/lifecycle"
	h "github.com/buildpack/pack/testhelpers"
)

func NewFakeBuilderImage(t *testing.T, name string, buildpacks []builder.BuildpackMetadata, config builder.Config) *fakes.Image {
	fakeBuilderImage := fakes.NewImage(name, "", "")
	h.AssertNil(t, fakeBuilderImage.SetLabel("io.buildpacks.stack.id", config.Stack.ID))
	h.AssertNil(t, fakeBuilderImage.SetEnv("CNB_USER_ID", "1234"))
	h.AssertNil(t, fakeBuilderImage.SetEnv("CNB_GROUP_ID", "4321"))
	metadata := builder.Metadata{
		Buildpacks: buildpacks,
		Groups:     config.Order.ToMetadata(),
		Stack: builder.StackMetadata{
			RunImage: builder.RunImageMetadata{
				Image:   config.Stack.RunImage,
				Mirrors: config.Stack.RunImageMirrors,
			},
		},
		Lifecycle: lifecycle.Metadata{
			Version: semver.MustParse(lifecycle.DefaultLifecycleVersion),
		},
	}
	label, err := json.Marshal(&metadata)
	h.AssertNil(t, err)
	h.AssertNil(t, fakeBuilderImage.SetLabel("io.buildpacks.builder.metadata", string(label)))
	return fakeBuilderImage
}
