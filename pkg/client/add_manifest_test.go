package client

import (
	"bytes"
	"context"
	"errors"
	"os"
	"testing"

	"github.com/buildpacks/imgutil"
	"github.com/buildpacks/imgutil/fakes"
	"github.com/buildpacks/pack/pkg/logging"
	"github.com/buildpacks/pack/pkg/testmocks"
	h "github.com/buildpacks/pack/testhelpers"
	"github.com/golang/mock/gomock"
	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/types"
	"github.com/heroku/color"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"
)

func TestAddManifest(t *testing.T) {
	color.Disable(true)
	defer color.Disable(false)
	spec.Run(t, "build", testAddManifest, spec.Report(report.Terminal{}))
}

func testAddManifest(t *testing.T, when spec.G, it spec.S) {
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
		it("should return an error if index doesn't exists locally", func() {
			prepareIndexWithoutLocallyExists(t, *mockIndexFactory)
			err = subject.AddManifest(
				context.TODO(),
				"pack/index",
				"pack/image",
				ManifestAddOptions{},
			)

			h.AssertEq(t, err.Error(), "index not found locally")
		})
		it("should add the given image", func() {
			digest, err := name.NewDigest("pack/image@sha256:15c46ced65c6abed6a27472a7904b04273e9a8091a5627badd6ff016ab073171")
			h.AssertNil(t, err)

			idx := prepareLoadIndex(t, *mockIndexFactory)
			err = subject.AddManifest(
				context.TODO(),
				"pack/index",
				digest.Identifier(),
				ManifestAddOptions{},
			)
			h.AssertNil(t, err)

			_, err = idx.OS(digest)
			h.AssertNil(t, err)
		})
		it("should add index with OS and Arch specific", func() {
			digest, err := name.NewDigest("pack/image@sha256:15c46ced65c6abed6a27472a7904b04273e9a8091a5627badd6ff016ab073171")
			h.AssertNil(t, err)

			idx := prepareLoadIndex(
				t, 
				*mockIndexFactory,
			)

			err = subject.AddManifest(
				context.TODO(),
				"pack/index",
				digest.Name(),
				ManifestAddOptions{
					OS: "some-os",
					OSArch: "some-arch",
				},
			)
			h.AssertNil(t, err)

			os, err := idx.OS(digest)
			h.AssertNil(t, err)
			h.AssertEq(t, os, "some-os")

			arch, err := idx.Architecture(digest)
			h.AssertNil(t, err)
			h.AssertEq(t, arch, "some-arch")
		})
		it("should add with variant", func() {
			digest, err := name.NewDigest("pack/image@sha256:15c46ced65c6abed6a27472a7904b04273e9a8091a5627badd6ff016ab073171")
			h.AssertNil(t, err)

			idx := prepareLoadIndex(
				t, 
				*mockIndexFactory, 
			)

			err = subject.AddManifest(
				context.TODO(),
				"pack/index",
				digest.Name(),
				ManifestAddOptions{
					OSVariant: "some-variant",
				},
			)
			h.AssertNil(t, err)

			variant, err := idx.Variant(digest)
			h.AssertNil(t, err)
			h.AssertEq(t, variant, "some-variant")
		})
		it("should add with osVersion", func() {
			digest, err := name.NewDigest("pack/image@sha256:15c46ced65c6abed6a27472a7904b04273e9a8091a5627badd6ff016ab073171")
			h.AssertNil(t, err)

			idx := prepareLoadIndex(
				t, 
				*mockIndexFactory, 
			)

			err = subject.AddManifest(
				context.TODO(),
				"pack/index",
				digest.Name(),
				ManifestAddOptions{
					OSVersion: "some-os-version",
				},
			)
			h.AssertNil(t, err)

			osVersion, err := idx.OSVersion(digest)
			h.AssertNil(t, err)
			h.AssertEq(t, osVersion, "some-os-version")
		})
		it("should add with features", func() {
			digest, err := name.NewDigest("pack/image@sha256:15c46ced65c6abed6a27472a7904b04273e9a8091a5627badd6ff016ab073171")
			h.AssertNil(t, err)

			idx := prepareLoadIndex(
				t, 
				*mockIndexFactory, 
			)

			err = subject.AddManifest(
				context.TODO(),
				"pack/index",
				digest.Name(),
				ManifestAddOptions{
					Features: []string{"some-features"},
				},
			)
			h.AssertNil(t, err)

			features, err := idx.Features(digest)
			h.AssertNil(t, err)
			h.AssertEq(t, features, []string{"some-features"})
		})
		it("should add with osFeatures", func() {
			digest, err := name.NewDigest("pack/image@sha256:15c46ced65c6abed6a27472a7904b04273e9a8091a5627badd6ff016ab073171")
			h.AssertNil(t, err)

			idx := prepareLoadIndex(
				t, 
				*mockIndexFactory, 
			)

			err = subject.AddManifest(
				context.TODO(),
				"pack/index",
				digest.Name(),
				ManifestAddOptions{
					Features: []string{"some-os-features"},
				},
			)
			h.AssertNil(t, err)

			osFeatures, err := idx.Features(digest)
			h.AssertNil(t, err)
			h.AssertEq(t, osFeatures, []string{"some-os-features"})
		})
		it("should add with annotations", func() {
			digest, err := name.NewDigest("pack/image@sha256:15c46ced65c6abed6a27472a7904b04273e9a8091a5627badd6ff016ab073171")
			h.AssertNil(t, err)

			idx := prepareLoadIndex(
				t, 
				*mockIndexFactory, 
			)

			err = subject.AddManifest(
				context.TODO(),
				"pack/index",
				digest.Name(),
				ManifestAddOptions{
					Annotations: map[string]string{"some-key":"some-value"},
				},
			)
			h.AssertNil(t, err)

			annos, err := idx.Annotations(digest)
			h.AssertNil(t, err)
			h.AssertEq(t, annos, map[string]string{"some-key":"some-value"})
		})
	})
}

func prepareIndexWithoutLocallyExists(t *testing.T, mockIndexFactory testmocks.MockIndexFactory) {	
	mockIndexFactory.
		EXPECT().
		LoadIndex(gomock.Any(), gomock.Any()).
		Return(nil, errors.New("index not found locally"))
}

func prepareLoadIndex(t *testing.T, mockIndexFactory testmocks.MockIndexFactory) (imgutil.ImageIndex) {	
	idx, err := fakes.NewIndex(types.OCIImageIndex, 1024, 1, 1, v1.Descriptor{})
	h.AssertNil(t, err)
	
	mockIndexFactory.
		EXPECT().
		LoadIndex(gomock.Any(), gomock.Any()).
		Return(idx, nil)

	return idx
}
