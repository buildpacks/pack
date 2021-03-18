package testmocks

import (
	"github.com/buildpacks/imgutil"
	"github.com/buildpacks/imgutil/fakes"
)

type SimpleMockImage struct {
	*fakes.Image
	EntrypointCall struct {
		callCount int
		Received  struct{}
		Returns   struct {
			StringArr []string
			Error     error
		}
		Stub func() ([]string, error)
	}
}

func NewSimpleImage(name, topLayerSha string, identifier imgutil.Identifier) *SimpleMockImage {
	return &SimpleMockImage{
		Image: fakes.NewImage(name, topLayerSha, identifier),
	}
}

func (m *SimpleMockImage) Entrypoint() ([]string, error) {
	if m.EntrypointCall.Stub != nil {
		return m.EntrypointCall.Stub()
	}
	return m.EntrypointCall.Returns.StringArr, m.EntrypointCall.Returns.Error
}
