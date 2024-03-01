package client

import (
	"bytes"
	"context"
	"os"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/heroku/color"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	"github.com/buildpacks/imgutil"

	"github.com/buildpacks/pack/pkg/logging"
	"github.com/buildpacks/pack/pkg/testmocks"
	h "github.com/buildpacks/pack/testhelpers"
)

func TestPushManifest(t *testing.T) {
	color.Disable(true)
	defer color.Disable(false)
	spec.Run(t, "build", testPushManifest, spec.Report(report.Terminal{}))
}

func testPushManifest(t *testing.T, when spec.G, it spec.S) {
	var (
		mockController   *gomock.Controller
		mockIndexFactory *testmocks.MockIndexFactory
		out              bytes.Buffer
		logger           logging.Logger
		subject          *Client
		err              error
		tmpDir           string
	)
	when("#Push", func() {
		it.Before(func() {
			logger = logging.NewLogWithWriters(&out, &out, logging.WithVerbose())
			mockController = gomock.NewController(t)
			mockIndexFactory = testmocks.NewMockIndexFactory(mockController)

			subject, err = NewClient(
				WithLogger(logger),
				WithIndexFactory(mockIndexFactory),
				WithExperimental(true),
				WithKeychain(authn.DefaultKeychain),
			)
			h.AssertSameInstance(t, mockIndexFactory, subject.indexFactory)
			h.AssertNil(t, err)
		})
		it.After(func() {
			mockController.Finish()
			h.AssertNil(t, os.RemoveAll(tmpDir))
		})
		it("should not have local image index", func() {
			prepareLoadIndexWithError(t, *mockIndexFactory)

			err := subject.PushManifest(context.TODO(), "some-index", PushManifestOptions{})
			h.AssertEq(t, err.Error(), imgutil.ErrNoImageOrIndexFoundWithGivenDigest("").Error())
		})
		it("should push index to registry", func() {
			prepareLoadIndex(t, *mockIndexFactory)

			err := subject.PushManifest(context.TODO(), "some-index", PushManifestOptions{})
			h.AssertNil(t, err)
		})
	})
}

func prepareLoadIndexWithError(t *testing.T, mockIndexFactory testmocks.MockIndexFactory) {
	mockIndexFactory.
		EXPECT().
		LoadIndex(gomock.Any(), gomock.Any()).
		Return(nil, imgutil.ErrNoImageOrIndexFoundWithGivenDigest(""))
}
