package client

import (
	"bytes"
	"context"
	"errors"
	"os"
	"testing"

	"github.com/buildpacks/imgutil"
	"github.com/buildpacks/imgutil/fakes"
	"github.com/golang/mock/gomock"
	"github.com/google/go-containerregistry/pkg/authn"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/types"
	"github.com/heroku/color"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	"github.com/buildpacks/pack/pkg/logging"
	"github.com/buildpacks/pack/pkg/testmocks"
	h "github.com/buildpacks/pack/testhelpers"
)

func TestInspectManifest(t *testing.T) {
	color.Disable(true)
	defer color.Disable(false)
	spec.Run(t, "build", testInspectManifest, spec.Report(report.Terminal{}))
}

func testInspectManifest(t *testing.T, when spec.G, it spec.S) {
	var (
		mockController   *gomock.Controller
		mockIndexFactory *testmocks.MockIndexFactory
		stdout           bytes.Buffer
		stderr           bytes.Buffer
		logger           logging.Logger
		subject          *Client
		err              error
		tmpDir           string
	)

	when("#Add", func() {
		it.Before(func() {
			logger = logging.NewLogWithWriters(&stdout, &stderr, logging.WithVerbose())
			mockController = gomock.NewController(t)
			mockIndexFactory = testmocks.NewMockIndexFactory(mockController)

			subject, err = NewClient(
				WithLogger(logger),
				WithIndexFactory(mockIndexFactory),
				WithExperimental(true),
				WithKeychain(authn.DefaultKeychain),
			)
			h.AssertSameInstance(t, mockIndexFactory, subject.indexFactory)
			h.AssertSameInstance(t, subject.logger, logger)
			h.AssertNil(t, err)
		})
		it.After(func() {
			mockController.Finish()
			h.AssertNil(t, os.RemoveAll(tmpDir))
		})
		it("should return an error when index not found", func() {
			prepareFindIndexWithError(t, *mockIndexFactory)

			err := subject.InspectManifest(
				context.TODO(),
				"some/name",
			)
			h.AssertEq(t, err.Error(), "index not found")
		})
		it("should return formatted IndexManifest", func() {
			prepareFindIndex(t, *mockIndexFactory)

			err := subject.InspectManifest(
				context.TODO(),
				"some/name",
			)
			h.AssertNil(t, err)
			h.AssertEq(t, stderr.String(), "")
		})
	})
}

func prepareFindIndexWithError(t *testing.T, mockIndexFactory testmocks.MockIndexFactory) {
	mockIndexFactory.
		EXPECT().
		FindIndex(gomock.Any(), gomock.Any()).
		AnyTimes().
		Return(nil, errors.New("index not found"))
}

func prepareFindIndex(t *testing.T, mockIndexFactory testmocks.MockIndexFactory) imgutil.ImageIndex {
	idx, err := fakes.NewIndex(types.OCIImageIndex, 1024, 1, 1, v1.Descriptor{})
	h.AssertNil(t, err)

	mockIndexFactory.
		EXPECT().
		FindIndex(gomock.Any(), gomock.Any()).
		AnyTimes().
		Return(idx, nil)

	return idx
}
