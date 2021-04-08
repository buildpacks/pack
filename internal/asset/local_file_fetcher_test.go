package asset_test

import (
	"context"
	"encoding/json"
	"github.com/buildpacks/pack/internal/asset"
	blob2 "github.com/buildpacks/pack/internal/blob"
	"github.com/buildpacks/pack/internal/dist"
	"github.com/buildpacks/pack/internal/ocipackage"
	h "github.com/buildpacks/pack/testhelpers"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"
)

func TestLocalFileFetcher(t *testing.T) {
	spec.Run(t, "LocalFileFetcher", testLocalFileFetcher, spec.Parallel(), spec.Report(report.Terminal{}))
}

func testLocalFileFetcher(t *testing.T, when spec.G, it spec.S) {
	var (
		subject            asset.LocalFileFetcher
		assert             = h.NewAssertionManager(t)
		expectedAssetCache *ocipackage.OciLayoutPackage
		tmpFile            *os.File
	)
	it.Before(func() {
		var err error
		subject = asset.NewLocalFileFetcher()

		testFile := filepath.Join("testdata", "fake-asset-cache.tar")
		testfd, err := os.Open(testFile)
		assert.Nil(err)

		expectedAssetCache, err = ocipackage.NewOCILayoutPackage(blob2.NewBlob(
			filepath.Join("testdata", "fake-asset-cache.tar"), blob2.RawOption),
		)
		assert.Nil(err)

		tmpFile, err = ioutil.TempFile("", "test-local-file-fetcher-abs")
		assert.Nil(err)

		_, err = io.Copy(tmpFile, testfd)
		assert.Nil(err)
	})
	it.After(func() {
		os.Remove(tmpFile.Name())
	})
	when("using an absolute path", func() {
		it("fetches asset at absolute path", func() {
			ociAssets, err := subject.FetchFileAssets(context.Background(), "/invalid-dir/:::", tmpFile.Name())
			assert.Nil(err)

			assert.Equal(len(ociAssets), 1)

			assertSameAssetLayers(t, ociAssets[0], expectedAssetCache)
		})
	})
	when("using a local path", func() {
		it("fetches asset relative to 'workingDir'", func() {
			dir := filepath.Dir(tmpFile.Name())
			fileName := filepath.Base(tmpFile.Name())
			ociAssets, err := subject.FetchFileAssets(context.Background(), dir, fileName)
			assert.Nil(err)

			assert.Equal(len(ociAssets), 1)

			assertSameAssetLayers(t, ociAssets[0], expectedAssetCache)
		})
	})

	when("Failure cases", func() {
		when("fetching a file that does not exist", func() {
			it("errors with helpful message", func() {
				impossibleFileName := "::::"
				_, err := subject.FetchFileAssets(context.Background(), "", impossibleFileName)
				assert.ErrorContains(err, `unable to fetch file asset "::::"`)
			})
		})
	})
}

func assertSameAssetLayers(t *testing.T, actual, expected asset.Readable) {
	t.Helper()

	expectedAssetLabel, err := expected.Label("io.buildpacks.asset.layers")
	h.AssertNil(t, err)

	expectedAssetMap := dist.AssetMap{}
	h.AssertNil(t, json.Unmarshal([]byte(expectedAssetLabel), &expectedAssetMap))

	actualAssetLabel, err := actual.Label("io.buildpacks.asset.layers")
	h.AssertNil(t, err)

	actualAssetMap := dist.AssetMap{}
	h.AssertNil(t, json.Unmarshal([]byte(actualAssetLabel), &actualAssetMap))

	h.AssertEq(t, actualAssetMap, expectedAssetMap)

	for _, asset := range actualAssetMap {
		actualLayer, err := actual.GetLayer(asset.LayerDiffID)
		h.AssertNil(t, err)

		actualContents, err := ioutil.ReadAll(actualLayer)
		h.AssertNil(t, err)

		expectedLayer, err := expected.GetLayer(asset.LayerDiffID)
		h.AssertNil(t, err)

		expectedContents, err := ioutil.ReadAll(expectedLayer)
		h.AssertNil(t, err)

		h.AssertEq(t, actualContents, expectedContents)
	}
}
