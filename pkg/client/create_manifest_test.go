package client

import (
	"bytes"
	"context"
	"errors"
	"os"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/google/go-containerregistry/pkg/authn"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/types"
	"github.com/heroku/color"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	"github.com/buildpacks/imgutil"
	"github.com/buildpacks/imgutil/fakes"

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
		subject          *Client
		err              error
		tmpDir           string
	)
	when("#CreateManifest", func() {
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
		it("create manifest", func() {
			prepareMockImageFactoryForValidCreateIndex(t, mockIndexFactory)
			err := subject.CreateManifest(
				context.TODO(),
				"pack/imgutil",
				[]string{"busybox:1.36-musl"},
				CreateManifestOptions{
					Insecure: true,
				},
			)
			h.AssertNil(t, err)
		})
		it("create manifests ignoring all option", func() {
			prepareMockImageFactoryForValidCreateIndex(t, mockIndexFactory)
			err := subject.CreateManifest(
				context.TODO(),
				"pack/imgutil",
				[]string{"busybox:1.36-musl"},
				CreateManifestOptions{
					Insecure: true,
					All:      true,
				},
			)
			h.AssertNil(t, err)
		})
		it("create manifests with all nested images", func() {
			prepareMockImageFactoryForValidCreateIndexWithAll(t, mockIndexFactory)
			err := subject.CreateManifest(
				context.TODO(),
				"pack/imgutil",
				[]string{"busybox:1.36-musl"},
				CreateManifestOptions{
					Insecure: true,
					All:      true,
				},
			)
			h.AssertNil(t, err)
		})
		it("return an error when index exists already", func() {
			prepareMockImageFactoryForInvalidCreateIndexExistsLoadIndex(t, mockIndexFactory)
			err := subject.CreateManifest(
				context.TODO(),
				"pack/imgutil",
				[]string{"busybox:1.36-musl"},
				CreateManifestOptions{
					Insecure: true,
				},
			)
			h.AssertEq(t, err.Error(), "exits in your local storage, use 'pack manifest remove' if you want to delete it")
		})
	})
}

func prepareMockImageFactoryForValidCreateIndex(t *testing.T, mockIndexFactory *testmocks.MockIndexFactory) {
	idx, err := fakes.NewIndex(types.OCIImageIndex, 1024, 1, 1, v1.Descriptor{})
	h.AssertNil(t, err)

	mockIndexFactory.EXPECT().
		CreateIndex(gomock.Any(), gomock.Any()).
		AnyTimes().
		Return(idx, err)
	mockIndexFactory.EXPECT().
		LoadIndex(gomock.Any(), gomock.Any()).
		AnyTimes().
		After(
			mockIndexFactory.EXPECT().
				LoadIndex(gomock.Any(), gomock.Any()).
				Times(1).
				Return(
					imgutil.ImageIndex(nil),
					errors.New("no image exists"),
				),
		).
		Return(idx, err)
}

func prepareMockImageFactoryForValidCreateIndexWithAll(t *testing.T, mockIndexFactory *testmocks.MockIndexFactory) {
	idx, err := fakes.NewIndex(types.OCIImageIndex, 1024, 1, 1, v1.Descriptor{})
	h.AssertNil(t, err)

	mockIndexFactory.EXPECT().
		CreateIndex(gomock.Any(), gomock.Any()).
		AnyTimes().
		Return(idx, err)
	mockIndexFactory.EXPECT().
		LoadIndex(gomock.Any(), gomock.Any()).
		AnyTimes().
		After(
			mockIndexFactory.EXPECT().
				LoadIndex(gomock.Any(), gomock.Any()).
				Times(1).
				Return(
					imgutil.ImageIndex(nil),
					errors.New("no image exists"),
				),
		).
		Return(idx, err)
}

func prepareMockImageFactoryForInvalidCreateIndexExistsLoadIndex(t *testing.T, mockIndexFactory *testmocks.MockIndexFactory) {
	idx, err := fakes.NewIndex(types.OCIImageIndex, 1024, 1, 1, v1.Descriptor{})
	h.AssertNil(t, err)

	mockIndexFactory.EXPECT().
		CreateIndex(gomock.Any(), gomock.Any()).
		AnyTimes().
		Return(idx, err)

	mockIndexFactory.EXPECT().
		LoadIndex(gomock.Any(), gomock.Any()).
		AnyTimes().
		Return(idx, err)
}
