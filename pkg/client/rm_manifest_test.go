package client

import (
	"bytes"
	"context"
	"os"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/heroku/color"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	"github.com/buildpacks/imgutil/fakes"

	"github.com/buildpacks/pack/pkg/logging"
	"github.com/buildpacks/pack/pkg/testmocks"
	h "github.com/buildpacks/pack/testhelpers"
)

func TestRemoveManifest(t *testing.T) {
	color.Disable(true)
	defer color.Disable(false)
	spec.Run(t, "build", testRemoveManifest, spec.Report(report.Terminal{}))
}

func testRemoveManifest(t *testing.T, when spec.G, it spec.S) {
	var (
		mockController   *gomock.Controller
		mockIndexFactory *testmocks.MockIndexFactory
		out              bytes.Buffer
		logger           logging.Logger
		subject          *Client
		err              error
		tmpDir           string
	)

	when("#Add", func() {
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
		it("should remove local index", func() {
			idx := prepareLoadIndex(t, *mockIndexFactory)
			imgIdx, ok := idx.(*fakes.Index)
			h.AssertEq(t, ok, true)

			mfest, err := imgIdx.IndexManifest()
			h.AssertNil(t, err)

			digest, err := name.NewDigest("some/repo@" + mfest.Manifests[0].Digest.String())
			h.AssertNil(t, err)

			errs := subject.RemoveManifest(context.TODO(), "some-index", []string{digest.Name()})
			h.AssertEq(t, len(errs), 0)
		})
		it("should remove image", func() {
			idx := prepareLoadIndex(t, *mockIndexFactory)
			imgIdx, ok := idx.(*fakes.Index)
			h.AssertEq(t, ok, true)

			mfest, err := imgIdx.IndexManifest()
			h.AssertNil(t, err)

			digest, err := name.NewDigest("some/repo@" + mfest.Manifests[0].Digest.String())
			h.AssertNil(t, err)

			errs := subject.RemoveManifest(context.TODO(), "some-index", []string{digest.Name()})
			h.AssertEq(t, len(errs), 0)
		})
	})
}
