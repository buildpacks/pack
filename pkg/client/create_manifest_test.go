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
		// fakeIndex        *fakes.Index
	)
	when("#CreateManifest", func() {
		var (
			// xdgPath = "xdgPath"
			// ops = []index.Option{
			// 	index.WithKeychain(authn.DefaultKeychain),
			// 	index.WithXDGRuntimePath(xdgPath),
			// }
			prepareMockImageFactoryForValidCreateIndex = func() {
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

			prepareMockImageFactoryForValidCreateIndexWithAll = func() {
				idx, err := fakes.NewIndex(types.OCIImageIndex, 1024, 1, 1, v1.Descriptor{}, fakes.WithIndex(true))
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

			prepareMockImageFactoryForInvalidCreateIndexExistsLoadIndex = func() {
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
		)
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
		when("should", func() {
			it("create manifest", func() {
				prepareMockImageFactoryForValidCreateIndex()
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
				prepareMockImageFactoryForValidCreateIndex()
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
				prepareMockImageFactoryForValidCreateIndexWithAll()
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
				prepareMockImageFactoryForInvalidCreateIndexExistsLoadIndex()
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
		})
	})
}
