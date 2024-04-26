package client

import (
	"bytes"
	"context"
	"os"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/v1/random"
	"github.com/heroku/color"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	"github.com/buildpacks/imgutil"

	ifakes "github.com/buildpacks/pack/internal/fakes"
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
	})
	it.After(func() {
		mockController.Finish()
		h.AssertNil(t, os.RemoveAll(tmpDir))
	})

	when("#CreateManifest", func() {
		when("index doesn't exists", func() {
			when("remote manifest exists", func() {
				it.Before(func() {
					fakeImage := setUpRemoteImageForIndex(t, nil)
					fakeImageFetcher.RemoteImages["index.docker.io/library/busybox:1.36-musl"] = fakeImage

					prepareMockImageFactoryForValidCreateIndex(t, mockIndexFactory)
				})

				when("no errors on save", func() {
					it("creates the index with the given manifest", func() {
						err = subject.CreateManifest(
							context.TODO(),
							CreateManifestOptions{
								IndexRepoName: "pack/imgutil",
								RepoNames:     []string{"busybox:1.36-musl"},
								Insecure:      true,
							},
						)
						h.AssertNil(t, err)
					})
				})
			})
		})

		when("index exists", func() {
			it.Before(func() {
				mockIndexFactory.EXPECT().
					Exists(gomock.Any()).AnyTimes().Return(true)
			})

			it("return an error when index exists already", func() {
				err = subject.CreateManifest(
					context.TODO(),
					CreateManifestOptions{
						IndexRepoName: "pack/imgutil",
						RepoNames:     []string{"busybox:1.36-musl"},
						Insecure:      true,
					},
				)
				h.AssertEq(t, err.Error(), "exits in your local storage, use 'pack manifest remove' if you want to delete it")
			})
		})
	})
}

func prepareMockImageFactoryForValidCreateIndex(t *testing.T, mockIndexFactory *testmocks.MockIndexFactory) {
	ridx, err := random.Index(1024, 1, 2)
	h.AssertNil(t, err)

	options := &imgutil.IndexOptions{
		BaseIndex: ridx,
	}
	idx, err := imgutil.NewCNBIndex("foo", *options)
	h.AssertNil(t, err)

	mockIndexFactory.EXPECT().
		Exists(gomock.Any()).AnyTimes().Return(false)

	mockIndexFactory.EXPECT().
		CreateIndex(gomock.Any(), gomock.Any()).
		AnyTimes().
		Return(idx, err)
}
