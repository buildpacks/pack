package client

import (
	"bytes"
	"context"
	"os"
	"testing"

	"github.com/buildpacks/imgutil"
	"github.com/golang/mock/gomock"
	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/heroku/color"
	"github.com/pkg/errors"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	ifakes "github.com/buildpacks/pack/internal/fakes"
	"github.com/buildpacks/pack/pkg/logging"
	"github.com/buildpacks/pack/pkg/testmocks"
	h "github.com/buildpacks/pack/testhelpers"
)

const invalidDigest = "sha256:d4707523ce6e12afdbe9a3be5ad69027150a834870ca0933baf7516dd1fe0f56"

func TestAnnotateManifest(t *testing.T) {
	color.Disable(true)
	defer color.Disable(false)
	// TODO I think we can make this test to run in parallel
	spec.Run(t, "build", testAnnotateManifest, spec.Report(report.Terminal{}))
}

func testAnnotateManifest(t *testing.T, when spec.G, it spec.S) {
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

		tmpDir, err = os.MkdirTemp("", "annotate-manifest-test")
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

	when("#AnnotateManifest", func() {
		var (
			digest        name.Digest
			idx           imgutil.ImageIndex
			indexRepoName string
		)
		when("index doesn't exists", func() {
			it.Before(func() {
				mockIndexFactory.EXPECT().LoadIndex(gomock.Any(), gomock.Any()).Return(nil, errors.New("index not found locally"))
			})

			it("should return an error", func() {
				err = subject.AnnotateManifest(
					context.TODO(),
					ManifestAnnotateOptions{
						IndexRepoName: "pack/index",
						RepoName:      "pack/image",
					},
				)
				h.AssertEq(t, err.Error(), "index not found locally")
			})
		})

		when("index exists", func() {
			when("no errors on save", func() {
				it.Before(func() {
					indexRepoName = h.NewRandomIndexRepoName()
					idx, digest = h.RandomCNBIndexAndDigest(t, indexRepoName, 1, 2)
					mockIndexFactory.EXPECT().LoadIndex(gomock.Eq(indexRepoName), gomock.Any()).Return(idx, nil)
					fakeImage := h.NewFakeWithRandomUnderlyingV1Image(t, digest)
					fakeImageFetcher.RemoteImages[digest.Name()] = fakeImage
				})

				it("should set OS for given image", func() {
					err = subject.AnnotateManifest(
						context.TODO(),
						ManifestAnnotateOptions{
							IndexRepoName: indexRepoName,
							RepoName:      digest.Name(),
							OS:            "some-os",
						},
					)
					h.AssertNil(t, err)

					os, err := idx.OS(digest)
					h.AssertNil(t, err)
					h.AssertEq(t, os, "some-os")
				})

				it("should set Arch for given image", func() {
					err = subject.AnnotateManifest(
						context.TODO(),
						ManifestAnnotateOptions{
							IndexRepoName: indexRepoName,
							RepoName:      digest.Name(),
							OSArch:        "some-arch",
						},
					)
					h.AssertNil(t, err)

					arch, err := idx.Architecture(digest)
					h.AssertNil(t, err)
					h.AssertEq(t, arch, "some-arch")
				})

				it("should set Variant for given image", func() {
					err = subject.AnnotateManifest(
						context.TODO(),
						ManifestAnnotateOptions{
							IndexRepoName: indexRepoName,
							RepoName:      digest.Name(),
							OSVariant:     "some-variant",
						},
					)
					h.AssertNil(t, err)

					variant, err := idx.Variant(digest)
					h.AssertNil(t, err)
					h.AssertEq(t, variant, "some-variant")
				})

				it("should set Annotations for given image", func() {
					err = subject.AnnotateManifest(
						context.TODO(),
						ManifestAnnotateOptions{
							IndexRepoName: indexRepoName,
							RepoName:      digest.Name(),
							Annotations:   map[string]string{"some-key": "some-value"},
						},
					)
					h.AssertNil(t, err)

					annos, err := idx.Annotations(digest)
					h.AssertNil(t, err)
					h.AssertEq(t, annos, map[string]string{"some-key": "some-value"})
				})

				it("should save annotated index", func() {
					var (
						fakeOS          = "some-os"
						fakeArch        = "some-arch"
						fakeVariant     = "some-variant"
						fakeAnnotations = map[string]string{"some-key": "some-value"}
					)

					err = subject.AnnotateManifest(
						context.TODO(),
						ManifestAnnotateOptions{
							IndexRepoName: indexRepoName,
							RepoName:      digest.Name(),
							OS:            fakeOS,
							OSArch:        fakeArch,
							OSVariant:     fakeVariant,
							Annotations:   fakeAnnotations,
						},
					)
					h.AssertNil(t, err)

					err = idx.SaveDir()
					h.AssertNil(t, err)

					os, err := idx.OS(digest)
					h.AssertNil(t, err)
					h.AssertEq(t, os, fakeOS)

					arch, err := idx.Architecture(digest)
					h.AssertNil(t, err)
					h.AssertEq(t, arch, fakeArch)

					variant, err := idx.Variant(digest)
					h.AssertNil(t, err)
					h.AssertEq(t, variant, fakeVariant)

					/* TODO Getters are still available in the imgutil.ImageIndex interface but we removed the Setters
					osVersion, err := idx.OSVersion(digest)
					h.AssertNil(t, err)
					h.AssertEq(t, osVersion, fakeVersion)

					osFeatures, err := idx.OSFeatures(digest)
					h.AssertNil(t, err)
					h.AssertEq(t, osFeatures, []string{"some-OSFeatures", "some-OSFeatures"})
					*/

					annos, err := idx.Annotations(digest)
					h.AssertNil(t, err)
					h.AssertEq(t, annos, fakeAnnotations)
				})
			})
		})

		when("return an error", func() {
			var nonExistentDigest string

			it.Before(func() {
				indexRepoName = h.NewRandomIndexRepoName()
				idx = h.RandomCNBIndex(t, indexRepoName, 1, 2)
				nonExistentDigest = "busybox@" + invalidDigest
				mockIndexFactory.EXPECT().LoadIndex(gomock.Eq(indexRepoName), gomock.Any()).Return(idx, nil)
			})

			it("has no image with given digest for Arch", func() {
				err = subject.AnnotateManifest(
					context.TODO(),
					ManifestAnnotateOptions{
						IndexRepoName: indexRepoName,
						RepoName:      nonExistentDigest,
						OSArch:        "some-arch",
					},
				)
				h.AssertNotNil(t, err)
			})
			it("has no image with given digest for Variant", func() {
				err = subject.AnnotateManifest(
					context.TODO(),
					ManifestAnnotateOptions{
						IndexRepoName: indexRepoName,
						RepoName:      nonExistentDigest,
						OSVariant:     "some-variant",
					},
				)
				h.AssertNotNil(t, err)
			})
			it("has no image with given digest for Annotations", func() {
				err = subject.AnnotateManifest(
					context.TODO(),
					ManifestAnnotateOptions{
						IndexRepoName: indexRepoName,
						RepoName:      nonExistentDigest,
						Annotations:   map[string]string{"some-key": "some-value"},
					},
				)
				h.AssertNotNil(t, err)
			})
		})
	})
}
