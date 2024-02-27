package client

import (
	"bytes"
	"context"
	"os"
	"testing"

	"github.com/buildpacks/imgutil"
	"github.com/buildpacks/imgutil/fakes"
	"github.com/golang/mock/gomock"
	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/heroku/color"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	"github.com/buildpacks/pack/pkg/logging"
	"github.com/buildpacks/pack/pkg/testmocks"
	h "github.com/buildpacks/pack/testhelpers"
)

func TestAnnotateManifest(t *testing.T) {
	color.Disable(true)
	defer color.Disable(false)
	spec.Run(t, "build", testAnnotateManifest, spec.Report(report.Terminal{}))
}

func testAnnotateManifest(t *testing.T, when spec.G, it spec.S) {
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
		when("successful when", func() {
			it("should return an error if index doesn't exists locally", func() {
				prepareIndexWithoutLocallyExists(t, *mockIndexFactory)
				err = subject.AnnotateManifest(
					context.TODO(),
					"pack/index",
					"pack/image",
					ManifestAnnotateOptions{},
				)

				h.AssertEq(t, err.Error(), "index not found locally")
			})
			it("should set OS for given image", func() {
				idx := prepareLoadIndex(t, *mockIndexFactory)
				imgIdx, ok := idx.(*fakes.Index)
				h.AssertEq(t, ok, true)

				mfest, err := imgIdx.IndexManifest()
				h.AssertNil(t, err)

				digest, err := name.NewDigest("some/repo@" + mfest.Manifests[0].Digest.String())
				h.AssertNil(t, err)

				err = subject.AnnotateManifest(
					context.TODO(),
					"some/repo",
					digest.Name(),
					ManifestAnnotateOptions{
						OS: "some-os",
					},
				)
				h.AssertNil(t, err)

				os, err := idx.OS(digest)
				h.AssertNil(t, err)
				h.AssertEq(t, os, "some-os")
			})
			it("should set Arch for given image", func() {
				idx := prepareLoadIndex(t, *mockIndexFactory)
				imgIdx, ok := idx.(*fakes.Index)
				h.AssertEq(t, ok, true)

				mfest, err := imgIdx.IndexManifest()
				h.AssertNil(t, err)

				digest, err := name.NewDigest("some/repo@" + mfest.Manifests[0].Digest.String())
				h.AssertNil(t, err)

				err = subject.AnnotateManifest(
					context.TODO(),
					"some/repo",
					digest.Name(),
					ManifestAnnotateOptions{
						OSArch: "some-arch",
					},
				)
				h.AssertNil(t, err)

				arch, err := idx.Architecture(digest)
				h.AssertNil(t, err)
				h.AssertEq(t, arch, "some-arch")
			})
			it("should set Variant for given image", func() {
				idx := prepareLoadIndex(t, *mockIndexFactory)
				imgIdx, ok := idx.(*fakes.Index)
				h.AssertEq(t, ok, true)

				mfest, err := imgIdx.IndexManifest()
				h.AssertNil(t, err)

				digest, err := name.NewDigest("some/repo@" + mfest.Manifests[0].Digest.String())
				h.AssertNil(t, err)

				err = subject.AnnotateManifest(
					context.TODO(),
					"some/repo",
					digest.Name(),
					ManifestAnnotateOptions{
						OSVariant: "some-variant",
					},
				)
				h.AssertNil(t, err)

				variant, err := idx.Variant(digest)
				h.AssertNil(t, err)
				h.AssertEq(t, variant, "some-variant")
			})
			it("should set OSVersion for given image", func() {
				idx := prepareLoadIndex(t, *mockIndexFactory)
				imgIdx, ok := idx.(*fakes.Index)
				h.AssertEq(t, ok, true)

				mfest, err := imgIdx.IndexManifest()
				h.AssertNil(t, err)

				digest, err := name.NewDigest("some/repo@" + mfest.Manifests[0].Digest.String())
				h.AssertNil(t, err)

				err = subject.AnnotateManifest(
					context.TODO(),
					"some/repo",
					digest.Name(),
					ManifestAnnotateOptions{
						OSVersion: "some-osVersion",
					},
				)
				h.AssertNil(t, err)

				osVersion, err := idx.OSVersion(digest)
				h.AssertNil(t, err)
				h.AssertEq(t, osVersion, "some-osVersion")
			})
			it("should set Features for given image", func() {
				idx := prepareLoadIndex(t, *mockIndexFactory)
				imgIdx, ok := idx.(*fakes.Index)
				h.AssertEq(t, ok, true)

				mfest, err := imgIdx.IndexManifest()
				h.AssertNil(t, err)

				digest, err := name.NewDigest("some/repo@" + mfest.Manifests[0].Digest.String())
				h.AssertNil(t, err)

				err = subject.AnnotateManifest(
					context.TODO(),
					"some/repo",
					digest.Name(),
					ManifestAnnotateOptions{
						Features: []string{"some-features"},
					},
				)
				h.AssertNil(t, err)

				features, err := idx.Features(digest)
				h.AssertNil(t, err)
				h.AssertEq(t, features, []string{"some-features"})
			})
			it("should set OSFeatures for given image", func() {
				idx := prepareLoadIndex(t, *mockIndexFactory)
				imgIdx, ok := idx.(*fakes.Index)
				h.AssertEq(t, ok, true)

				mfest, err := imgIdx.IndexManifest()
				h.AssertNil(t, err)

				digest, err := name.NewDigest("some/repo@" + mfest.Manifests[0].Digest.String())
				h.AssertNil(t, err)

				err = subject.AnnotateManifest(
					context.TODO(),
					"some/repo",
					digest.Name(),
					ManifestAnnotateOptions{
						OSFeatures: []string{"some-osFeatures"},
					},
				)
				h.AssertNil(t, err)

				osFeatures, err := idx.OSFeatures(digest)
				h.AssertNil(t, err)
				h.AssertEq(t, osFeatures, []string{"some-osFeatures"})
			})
			it("should set URLs for given image", func() {
				idx := prepareLoadIndex(t, *mockIndexFactory)
				imgIdx, ok := idx.(*fakes.Index)
				h.AssertEq(t, ok, true)

				mfest, err := imgIdx.IndexManifest()
				h.AssertNil(t, err)

				digest, err := name.NewDigest("some/repo@" + mfest.Manifests[0].Digest.String())
				h.AssertNil(t, err)

				err = subject.AnnotateManifest(
					context.TODO(),
					"some/repo",
					digest.Name(),
					ManifestAnnotateOptions{
						URLs: []string{"some-urls"},
					},
				)
				h.AssertNil(t, err)

				urls, err := idx.URLs(digest)
				h.AssertNil(t, err)
				h.AssertEq(t, urls, []string{"some-urls"})
			})
			it("should set Annotations for given image", func() {
				idx := prepareLoadIndex(t, *mockIndexFactory)
				imgIdx, ok := idx.(*fakes.Index)
				h.AssertEq(t, ok, true)

				mfest, err := imgIdx.IndexManifest()
				h.AssertNil(t, err)

				digest, err := name.NewDigest("some/repo@" + mfest.Manifests[0].Digest.String())
				h.AssertNil(t, err)

				err = subject.AnnotateManifest(
					context.TODO(),
					"some/repo",
					digest.Name(),
					ManifestAnnotateOptions{
						Annotations: map[string]string{"some-key": "some-value"},
					},
				)
				h.AssertNil(t, err)

				annos, err := idx.Annotations(digest)
				h.AssertNil(t, err)
				h.AssertEq(t, annos, map[string]string{"some-key": "some-value"})
			})
		})
		when("return an error when", func() {
			it("has no Index locally by given Name", func() {
				prepareIndexWithoutLocallyExists(t, *mockIndexFactory)
				err = subject.AnnotateManifest(
					context.TODO(),
					"some/repo",
					"",
					ManifestAnnotateOptions{},
				)
				h.AssertEq(t, err.Error(), "index not found locally")
			})
			it("has no image with given digest for OS", func() {
				prepareLoadIndex(t, *mockIndexFactory)
				err = subject.AnnotateManifest(
					context.TODO(),
					"some/repo",
					"busybox@sha256:d4707523ce6e12afdbe9a3be5ad69027150a834870ca0933baf7516dd1fe0f56",
					ManifestAnnotateOptions{
						OS: "some-os",
					},
				)
				h.AssertEq(t, err.Error(), imgutil.ErrNoImageOrIndexFoundWithGivenDigest.Error())
			})
			it("has no image with given digest for Arch", func() {
				prepareLoadIndex(t, *mockIndexFactory)
				err = subject.AnnotateManifest(
					context.TODO(),
					"some/repo",
					"busybox@sha256:d4707523ce6e12afdbe9a3be5ad69027150a834870ca0933baf7516dd1fe0f56",
					ManifestAnnotateOptions{
						OSArch: "some-arch",
					},
				)
				h.AssertEq(t, err.Error(), imgutil.ErrNoImageOrIndexFoundWithGivenDigest.Error())
			})
			it("has no image with given digest for Variant", func() {
				prepareLoadIndex(t, *mockIndexFactory)
				err = subject.AnnotateManifest(
					context.TODO(),
					"some/repo",
					"busybox@sha256:d4707523ce6e12afdbe9a3be5ad69027150a834870ca0933baf7516dd1fe0f56",
					ManifestAnnotateOptions{
						OSVariant: "some-variant",
					},
				)
				h.AssertEq(t, err.Error(), imgutil.ErrNoImageOrIndexFoundWithGivenDigest.Error())
			})
			it("has no image with given digest for osVersion", func() {
				prepareLoadIndex(t, *mockIndexFactory)
				err = subject.AnnotateManifest(
					context.TODO(),
					"some/repo",
					"busybox@sha256:d4707523ce6e12afdbe9a3be5ad69027150a834870ca0933baf7516dd1fe0f56",
					ManifestAnnotateOptions{
						OSVersion: "some-osVersion",
					},
				)
				h.AssertEq(t, err.Error(), imgutil.ErrNoImageOrIndexFoundWithGivenDigest.Error())
			})
			it("has no image with given digest for Features", func() {
				prepareLoadIndex(t, *mockIndexFactory)
				err = subject.AnnotateManifest(
					context.TODO(),
					"some/repo",
					"busybox@sha256:d4707523ce6e12afdbe9a3be5ad69027150a834870ca0933baf7516dd1fe0f56",
					ManifestAnnotateOptions{
						Features: []string{"some-features"},
					},
				)
				h.AssertEq(t, err.Error(), imgutil.ErrNoImageOrIndexFoundWithGivenDigest.Error())
			})
			it("has no image with given digest for OSFeatures", func() {
				prepareLoadIndex(t, *mockIndexFactory)
				err = subject.AnnotateManifest(
					context.TODO(),
					"some/repo",
					"busybox@sha256:d4707523ce6e12afdbe9a3be5ad69027150a834870ca0933baf7516dd1fe0f56",
					ManifestAnnotateOptions{
						OSFeatures: []string{"some-osFeatures"},
					},
				)
				h.AssertEq(t, err.Error(), imgutil.ErrNoImageOrIndexFoundWithGivenDigest.Error())
			})
			it("has no image with given digest for URLs", func() {
				prepareLoadIndex(t, *mockIndexFactory)
				err = subject.AnnotateManifest(
					context.TODO(),
					"some/repo",
					"busybox@sha256:d4707523ce6e12afdbe9a3be5ad69027150a834870ca0933baf7516dd1fe0f56",
					ManifestAnnotateOptions{
						URLs: []string{"some-urls"},
					},
				)
				h.AssertEq(t, err.Error(), imgutil.ErrNoImageOrIndexFoundWithGivenDigest.Error())
			})
			it("has no image with given digest for Annotations", func() {
				prepareLoadIndex(t, *mockIndexFactory)
				err = subject.AnnotateManifest(
					context.TODO(),
					"some/repo",
					"busybox@sha256:d4707523ce6e12afdbe9a3be5ad69027150a834870ca0933baf7516dd1fe0f56",
					ManifestAnnotateOptions{
						Annotations: map[string]string{"some-key": "some-value"},
					},
				)
				h.AssertEq(t, err.Error(), imgutil.ErrNoImageOrIndexFoundWithGivenDigest.Error())
			})
		})
	})
}
