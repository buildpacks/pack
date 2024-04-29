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

	"github.com/buildpacks/pack/pkg/logging"
	"github.com/buildpacks/pack/pkg/testmocks"
	h "github.com/buildpacks/pack/testhelpers"
)

func TestDeleteManifest(t *testing.T) {
	color.Disable(true)
	defer color.Disable(false)
	spec.Run(t, "build", testDeleteManifest, spec.Report(report.Terminal{}))
}

func testDeleteManifest(t *testing.T, when spec.G, it spec.S) {
	var (
		mockController   *gomock.Controller
		mockIndexFactory *testmocks.MockIndexFactory
		out              bytes.Buffer
		logger           logging.Logger
		subject          *Client
		err              error
		tmpDir           string
	)

	it.Before(func() {
		logger = logging.NewLogWithWriters(&out, &out, logging.WithVerbose())
		mockController = gomock.NewController(t)
		mockIndexFactory = testmocks.NewMockIndexFactory(mockController)

		tmpDir, err = os.MkdirTemp("", "remove-manifest-test")
		h.AssertNil(t, err)
		os.Setenv("XDG_RUNTIME_DIR", tmpDir)

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

	when("#DeleteManifest", func() {
		when("index doesn't exists", func() {
			it.Before(func() {
				prepareIndexWithoutLocallyExists(*mockIndexFactory)
			})
			it("should return an error when index is already deleted", func() {
				errs := subject.DeleteManifest(context.TODO(), []string{"pack/none-existent-index"})
				h.AssertNotEq(t, len(errs), 0)
			})
		})

		when("index exists", func() {
			var index *h.MockImageIndex
			it.Before(func() {
				index = h.NewMockImageIndex(t, "some-index", 1, 2)
				mockIndexFactory.EXPECT().LoadIndex(gomock.Eq("some-index"), gomock.Any()).Return(index, nil)
			})

			it("should delete local index", func() {
				errs := subject.DeleteManifest(context.TODO(), []string{"some-index"})
				h.AssertEq(t, len(errs), 0)
				h.AssertTrue(t, index.DeleteDirCalled)
			})
		})
	})
}
