package asset_test

import (
	"errors"
	"github.com/buildpacks/pack/internal/asset"
	"github.com/buildpacks/pack/internal/asset/fakes"
	"github.com/buildpacks/pack/internal/asset/testmocks"
	"github.com/buildpacks/pack/internal/blob"
	"github.com/buildpacks/pack/internal/dist"
	h "github.com/buildpacks/pack/testhelpers"
	"github.com/docker/docker/pkg/archive"
	"github.com/golang/mock/gomock"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"
)

func TestBlob(t *testing.T) {
	spec.Run(t, "BlobTest", testBlob, spec.Parallel(), spec.Report(report.Terminal{}))
}

func testBlob(t *testing.T, when spec.G, it spec.S) {
	var (
		assert = h.NewAssertionManager(t)
		mockController *gomock.Controller
		mockBlob       *testmocks.MockBlob
	)
	it.Before(func() {
		mockController = gomock.NewController(t)
		mockBlob = testmocks.NewMockBlob(mockController)
	})
	when("FromRawBlob", func() {
		it("successfully creates a readable asset blob", func() {

			fakeBlob := fakes.NewFakeBlob("blob contents")
			fakeAsset := dist.Asset{
				Sha256:      "some-sha256",
				ID:          "some-id",
				Version:     "1.2.3",
				Name:        "Some Fake Asset Name",
				Stacks:      []string{"first-stack", "second-stack"},
			}
			subject := asset.FromRawBlob(fakeAsset, fakeBlob)

			EqualBlobContents(t, subject, fakeBlob)

			assert.Equal(subject.AssetDescriptor(), fakeAsset)
			assert.Equal(subject.Size(), int64(len("blob contents")))
		})
	})

	when("ExtractFromLayer", func() {
		var layerBlob dist.Blob
		it.Before(func() {
			layerBlob = blob.NewBlob(filepath.Join("testdata", "fake-asset-layer.tar"))
		})
		it("creates a readable asset blob", func() {
			fakeAsset := dist.Asset{
				Sha256:      "71415dc9d46dd5722974eb79c14510dc1c0038dd3613afc85e911336b5b11c43",
				ID:          "some-fake-asset",
				Version:     "1.2.3",
				Stacks:      []string{"io.buildpacks.stacks.bionic"},
			}
			subject, err := asset.ExtractFromLayer(fakeAsset, layerBlob)

			subjectReader, err := subject.Open()
			assert.Nil(err)

			contents, err := ioutil.ReadAll(subjectReader)
			assert.Nil(err)

			assert.Equal(string(contents), "fake asset with some contents\n")
		})
	})

	when("error cases", func() {
		when("#ExtractFromLayer", func() {
			when("unable to open blob", func() {
				it("errors with a helpful message", func() {
					mockBlob.EXPECT().Open().Return(nil, errors.New("opening error"))
					_, err := asset.ExtractFromLayer(dist.Asset{}, mockBlob)

					assert.ErrorContains(err, "unable to open blob for extraction")
				})
			})
			when("unable to find asset in blob", func() {
				var tmpDir string
				it.Before(func() {
					var err error
					tmpDir, err = ioutil.TempDir("", "blob-test")
					assert.Nil(err)
				})
				it.After(func() {
					os.RemoveAll(tmpDir)
				})
				it("errors with a helpful message", func() {
					emptyArchive, err := archive.Tar(tmpDir, archive.Uncompressed)
					assert.Nil(err)

					mockBlob.EXPECT().Open().Return(emptyArchive, nil)
					_, err = asset.ExtractFromLayer(dist.Asset{Sha256: "help I lost my sha"}, mockBlob)

					assert.ErrorContains(err, `unable to find asset with sha256: "help I lost my sha" in blob`)

				})
			})
		})
	})
}

func EqualBlobContents(t *testing.T, actual, expected dist.Blob) {
	actualReader, err := actual.Open()
	h.AssertNil(t, err)
	actualContents, err := ioutil.ReadAll(actualReader)
	h.AssertNil(t, err)

	expectedReader, err := expected.Open()
	h.AssertNil(t, err)
	expectedContents, err := ioutil.ReadAll(expectedReader)
	h.AssertNil(t, err)

	h.AssertEq(t, string(actualContents), string(expectedContents))
}