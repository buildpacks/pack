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
	"github.com/google/go-containerregistry/pkg/v1/random"
	"github.com/heroku/color"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	ifakes "github.com/buildpacks/pack/internal/fakes"
	"github.com/buildpacks/pack/pkg/logging"
	"github.com/buildpacks/pack/pkg/testmocks"
	h "github.com/buildpacks/pack/testhelpers"
)

func TestAddManifest(t *testing.T) {
	color.Disable(true)
	defer color.Disable(false)

	// TODO I think we can make this test to run in parallel
	spec.Run(t, "build", testAddManifest, spec.Report(report.Terminal{}))
}

func testAddManifest(t *testing.T, when spec.G, it spec.S) {
	var (
		mockController   *gomock.Controller
		mockIndexFactory *testmocks.MockIndexFactory
		fakeImageFetcher *ifakes.FakeImageFetcher
		out              bytes.Buffer
		logger           logging.Logger
		subject          *Client
		err              error
		tmpDir           string
	)

	it.Before(func() {
		fakeImageFetcher = ifakes.NewFakeImageFetcher()
		logger = logging.NewLogWithWriters(&out, &out, logging.WithVerbose())
		mockController = gomock.NewController(t)
		mockIndexFactory = testmocks.NewMockIndexFactory(mockController)

		tmpDir, err = os.MkdirTemp("", "add-manifest-test")
		h.AssertNil(t, err)
		os.Setenv("XDG_RUNTIME_DIR", tmpDir)

		subject, err = NewClient(
			WithLogger(logger),
			WithFetcher(fakeImageFetcher),
			WithIndexFactory(mockIndexFactory),
			WithExperimental(true),
			WithKeychain(authn.DefaultKeychain),
		)
		h.AssertSameInstance(t, mockIndexFactory, subject.indexFactory)
		h.AssertNil(t, err)

		// Create a remote image to be fetched when adding to the image index
		fakeImage := setUpRemoteImageForIndex(t, nil)
		fakeImageFetcher.RemoteImages["index.docker.io/pack/image:latest"] = fakeImage
	})
	it.After(func() {
		mockController.Finish()
		h.AssertNil(t, os.RemoveAll(tmpDir))
	})

	when("#AddManifest", func() {
		when("index doesn't exists", func() {
			it.Before(func() {
				prepareIndexWithoutLocallyExists(*mockIndexFactory)
			})

			it("should return an error", func() {
				err = subject.AddManifest(
					context.TODO(),
					ManifestAddOptions{
						IndexRepoName: "pack/none-existent-index",
						RepoName:      "pack/image",
					},
				)
				h.AssertError(t, err, "index not found locally")
			})
		})

		when("index exists", func() {
			when("no errors on save", func() {
				it.Before(func() {
					prepareLoadIndex(t, "pack/index", *mockIndexFactory)
				})

				it("adds the given image", func() {
					err = subject.AddManifest(
						context.TODO(),
						ManifestAddOptions{
							IndexRepoName: "pack/index",
							RepoName:      "pack/image",
						},
					)
					h.AssertNil(t, err)
					h.AssertContains(t, out.String(), "successfully added to index: 'pack/image'")
				})

				it("error when invalid manifest reference name is used", func() {
					err = subject.AddManifest(
						context.TODO(),
						ManifestAddOptions{
							IndexRepoName: "pack/index",
							RepoName:      "pack@@image",
						},
					)
					h.AssertNotNil(t, err)
					h.AssertError(t, err, "is not a valid manifest reference")
				})

				it("error when manifest reference doesn't exist in a registry", func() {
					err = subject.AddManifest(
						context.TODO(),
						ManifestAddOptions{
							IndexRepoName: "pack/index",
							RepoName:      "pack/image-not-found",
						},
					)
					h.AssertNotNil(t, err)
					h.AssertError(t, err, "does not exist in registry")
				})
			})

			when("errors on save", func() {
				it.Before(func() {
					prepareLoadIndexWithErrorOnSave(t, "pack/index-error-on-saved", *mockIndexFactory)
				})

				it("error when manifest couldn't be saved locally", func() {
					err = subject.AddManifest(
						context.TODO(),
						ManifestAddOptions{
							IndexRepoName: "pack/index-error-on-saved",
							RepoName:      "pack/image",
						},
					)
					h.AssertNotNil(t, err)
					h.AssertError(t, err, "could not be saved in the local storage")
				})
			})
		})
	})
}

func setUpRemoteImageForIndex(t *testing.T, identifier imgutil.Identifier) *testImage {
	fakeCNBImage := fakes.NewImage("pack/image", "", identifier)
	underlyingImage, err := random.Image(1024, 1)
	h.AssertNil(t, err)
	return &testImage{
		Image:           fakeCNBImage,
		underlyingImage: underlyingImage,
	}
}

func prepareIndexWithoutLocallyExists(mockIndexFactory testmocks.MockIndexFactory) {
	mockIndexFactory.
		EXPECT().
		LoadIndex(gomock.Any(), gomock.Any()).
		Return(nil, errors.New("index not found locally"))
}

func prepareLoadIndex(t *testing.T, repoName string, mockIndexFactory testmocks.MockIndexFactory) imgutil.ImageIndex {
	idx := h.RandomCNBIndex(t, repoName, 1, 2)
	mockIndexFactory.
		EXPECT().
		LoadIndex(gomock.Eq(repoName), gomock.Any()).
		Return(idx, nil).
		AnyTimes()

	return idx
}

func prepareLoadIndexWithErrorOnSave(t *testing.T, repoName string, mockIndexFactory testmocks.MockIndexFactory) imgutil.ImageIndex {
	cnbIdx := h.RandomCNBIndex(t, repoName, 1, 2)
	idx := &h.MockImageIndex{
		CNBIndex:    *cnbIdx,
		ErrorOnSave: true,
	}
	mockIndexFactory.
		EXPECT().
		LoadIndex(gomock.Eq(repoName), gomock.Any()).
		Return(idx, nil).
		AnyTimes()

	return idx
}

type testImage struct {
	*fakes.Image
	underlyingImage v1.Image
}

func (t *testImage) UnderlyingImage() v1.Image {
	return t.underlyingImage
}
