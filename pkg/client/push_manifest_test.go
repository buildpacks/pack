package client

import (
	"bytes"
	"os"
	"testing"

	"github.com/buildpacks/imgutil"
	"github.com/golang/mock/gomock"
	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/heroku/color"
	"github.com/pkg/errors"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

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

	when("#PushManifest", func() {
		when("index exists locally", func() {
			var index *testPushIndex

			it.Before(func() {
				index = prepareLoadIndexWithMockPush(t, "some-index", *mockIndexFactory)
			})
			it("should push index to registry", func() {
				err = subject.PushManifest(PushManifestOptions{
					IndexRepoName: "some-index",
				})
				h.AssertNil(t, err)
				h.AssertTrue(t, index.PushCalled)
			})
		})

		when("index doesn't exist locally", func() {
			it.Before(func() {
				prepareLoadIndexWithError(*mockIndexFactory)
			})

			it("should not have local image index", func() {
				err = subject.PushManifest(PushManifestOptions{
					IndexRepoName: "some-index",
				})
				h.AssertNotNil(t, err)
			})
		})
	})
}

func prepareLoadIndexWithError(mockIndexFactory testmocks.MockIndexFactory) {
	mockIndexFactory.
		EXPECT().
		LoadIndex(gomock.Any(), gomock.Any()).
		Return(nil, errors.New("ErrNoImageOrIndexFoundWithGivenDigest"))
}

func prepareLoadIndexWithMockPush(t *testing.T, repoName string, mockIndexFactory testmocks.MockIndexFactory) *testPushIndex {
	cnbIdx := randomCNBIndex(t, repoName)
	idx := &testPushIndex{
		CNBIndex: *cnbIdx,
	}
	mockIndexFactory.
		EXPECT().
		LoadIndex(gomock.Eq(repoName), gomock.Any()).
		Return(idx, nil).
		AnyTimes()

	return idx
}

type testPushIndex struct {
	imgutil.CNBIndex
	PushCalled bool
}

func (i *testPushIndex) Push(_ ...imgutil.IndexOption) error {
	i.PushCalled = true
	return nil
}
