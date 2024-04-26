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
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	ifakes "github.com/buildpacks/pack/internal/fakes"
	"github.com/buildpacks/pack/pkg/logging"
	"github.com/buildpacks/pack/pkg/testmocks"
	h "github.com/buildpacks/pack/testhelpers"
)

const digestStr = "sha256:d4707523ce6e12afdbe9a3be5ad69027150a834870ca0933baf7516dd1fe0f56"

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
		when("index doesn't exists", func() {
			it.Before(func() {
				prepareIndexWithoutLocallyExists(*mockIndexFactory)
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
				var digest name.Digest
				var idx imgutil.ImageIndex

				it.Before(func() {
					idx = prepareLoadIndex(t, "some/repo", *mockIndexFactory)
					imgIdx, ok := idx.(*imgutil.CNBIndex)
					h.AssertEq(t, ok, true)

					mfest, err := imgIdx.IndexManifest()
					h.AssertNil(t, err)

					digest, err = name.NewDigest("some/repo@" + mfest.Manifests[0].Digest.String())
					h.AssertNil(t, err)

					fakeImage := setUpRemoteImageForIndex(t, digest)
					fakeImageFetcher.RemoteImages[digest.Name()] = fakeImage
				})

				it("should set OS for given image", func() {
					err = subject.AnnotateManifest(
						context.TODO(),
						ManifestAnnotateOptions{
							IndexRepoName: "some/repo",
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
							IndexRepoName: "some/repo",
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
							IndexRepoName: "some/repo",
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
							IndexRepoName: "some/repo",
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
							IndexRepoName: "some/repo",
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
			it("has no Index locally by given Name", func() {
				prepareIndexWithoutLocallyExists(*mockIndexFactory)
				err = subject.AnnotateManifest(
					context.TODO(),
					ManifestAnnotateOptions{
						IndexRepoName: "some/repo",
						RepoName:      "",
					},
				)
				h.AssertEq(t, err.Error(), "index not found locally")
			})
			it("has no image with given digest for OS", func() {
				prepareLoadIndex(t, "some/repo", *mockIndexFactory)

				err = subject.AnnotateManifest(
					context.TODO(),
					ManifestAnnotateOptions{
						IndexRepoName: "some/repo",
						RepoName:      "busybox@" + digestStr,
						OS:            "some-os",
					},
				)
				h.AssertNotNil(t, err)
			})
			it("has no image with given digest for Arch", func() {
				prepareLoadIndex(t, "some/repo", *mockIndexFactory)
				err = subject.AnnotateManifest(
					context.TODO(),
					ManifestAnnotateOptions{
						IndexRepoName: "some/repo",
						RepoName:      "busybox@" + digestStr,
						OSArch:        "some-arch",
					},
				)
				h.AssertNotNil(t, err)
			})
			it("has no image with given digest for Variant", func() {
				prepareLoadIndex(t, "some/repo", *mockIndexFactory)
				err = subject.AnnotateManifest(
					context.TODO(),
					ManifestAnnotateOptions{
						IndexRepoName: "some/repo",
						RepoName:      "busybox@" + digestStr,
						OSVariant:     "some-variant",
					},
				)
				h.AssertNotNil(t, err)
			})
			it("has no image with given digest for Annotations", func() {
				prepareLoadIndex(t, "some/repo", *mockIndexFactory)
				err = subject.AnnotateManifest(
					context.TODO(),
					ManifestAnnotateOptions{
						IndexRepoName: "some/repo",
						RepoName:      "busybox@" + digestStr,
						Annotations:   map[string]string{"some-key": "some-value"},
					},
				)
				h.AssertNotNil(t, err)
			})
		})
	})
}
