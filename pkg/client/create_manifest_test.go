package client_test

import (
	"bytes"
	"context"
	"os"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/heroku/color"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	"github.com/buildpacks/pack/pkg/client"
	"github.com/buildpacks/pack/pkg/logging"
	"github.com/buildpacks/pack/pkg/testmocks"
	h "github.com/buildpacks/pack/testhelpers"
)

func TestCreateManifest(t *testing.T) {
	color.Disable(true)
	defer color.Disable(false)
	spec.Run(t, "build", testCreateManifest, spec.Report(report.Terminal{}))
}

func testCreateManifest(t *testing.T, when spec.G, it spec.S) {
	var (
		mockController   *gomock.Controller
		mockIndexFactory *testmocks.MockIndexFactory
		out              bytes.Buffer
		logger           logging.Logger
		subject          *client.Client
		err              error
		tmpDir           string
		// fakeIndex        *fakes.Index
	)
	when("#CreateManifest", func() {
		it.Before(func() {
			logger = logging.NewLogWithWriters(&out, &out, logging.WithVerbose())
			mockController = gomock.NewController(t)
			mockIndexFactory = testmocks.NewMockIndexFactory(mockController)

			// fakeIndex, err = fakes.NewIndex(types.OCIImageIndex, 1024, 1, 1, v1.Descriptor{})
			// h.AssertNil(t, err)

			subject, err = client.NewClient(
				client.WithLogger(logger),
				client.WithIndexFactory(mockIndexFactory),
			)
			h.AssertNil(t, err)
		})
		it.After(func() {
			mockController.Finish()
			h.AssertNil(t, os.RemoveAll(tmpDir))
		})
		when("", func() {
			it("should create manifest", func() {
				err := subject.CreateManifest(
					context.TODO(),
					"pack/imgutil",
					[]string{"busybox:1.36-musl"},
					client.CreateManifestOptions{
						Insecure: true,
					},
				)
				h.AssertNil(t, err)
			})
		})
	})
}
