package asset_test

import (
	"encoding/json"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/buildpacks/imgutil"
	"github.com/buildpacks/imgutil/fakes"
	"github.com/golang/mock/gomock"
	"github.com/pkg/errors"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	pubcfg "github.com/buildpacks/pack/config"
	"github.com/buildpacks/pack/internal/asset"
	"github.com/buildpacks/pack/internal/asset/testmocks"
	blob2 "github.com/buildpacks/pack/internal/blob"
	"github.com/buildpacks/pack/internal/dist"
	"github.com/buildpacks/pack/internal/ocipackage"
	h "github.com/buildpacks/pack/testhelpers"
)

func TestFetcher(t *testing.T) {
	spec.Run(t, "TestFetcher", testFetcher, spec.Parallel(), spec.Report(report.Terminal{}))
}

func testFetcher(t *testing.T, when spec.G, it spec.S) {
	var (
		mockController   *gomock.Controller
		mockFileFetcher  *testmocks.MockFileFetcher
		mockURIFetcher   *testmocks.MockURIFetcher
		mockImageFetcher *testmocks.MockImageFetcher
		subject          asset.Fetcher
	)

	it.Before(func() {
		mockController = gomock.NewController(t)
		mockFileFetcher = testmocks.NewMockFileFetcher(mockController)
		mockURIFetcher = testmocks.NewMockURIFetcher(mockController)
		mockImageFetcher = testmocks.NewMockImageFetcher(mockController)
		subject = asset.NewFetcher(mockFileFetcher, mockURIFetcher, mockImageFetcher)
	})

	when("#FetchAssets", func() {
		when("fetching image asset", func() {
			var tmpDir string
			it.Before(func() {
				var err error
				tmpDir, err = ioutil.TempDir("", "fetcher-tests")
				h.AssertNil(t, err)
			})
			it.After(func() {
				os.RemoveAll(tmpDir)
			})
			it("fetches image with default pull policy", func() {
				assetName := "some-org/some-repo"
				fakeImage, err := newFakeAssetImage("some-contents", tmpDir)
				h.AssertNil(t, err)

				mockImageFetcher.EXPECT().FetchImageAssets(gomock.Any(), pubcfg.PullIfNotPresent, assetName).
					Return([]imgutil.Image{fakeImage}, nil)

				actualAssets, err := subject.FetchAssets([]string{assetName})
				h.AssertNil(t, err)

				h.AssertEq(t, len(actualAssets), 1)
				assertSameAssetLayers(t, actualAssets[0], fakeImage)

			})
			when("WithPullPolicy Option", func() {
				it("fetches asset with provided pull policy", func() {
					assetName := "some-org/some-repo"
					fakeImage, err := newFakeAssetImage("some-contents", tmpDir)
					h.AssertNil(t, err)

					mockImageFetcher.EXPECT().FetchImageAssets(gomock.Any(), pubcfg.PullAlways, assetName).
						Return([]imgutil.Image{fakeImage}, nil)

					actualAssets, err := subject.FetchAssets([]string{assetName}, asset.WithPullPolicy(pubcfg.PullAlways))
					h.AssertNil(t, err)

					h.AssertEq(t, len(actualAssets), 1)
					assertSameAssetLayers(t, actualAssets[0], fakeImage)
				})
			})
		})

		when("fetching file asset", func() {
			var (
				expectedAssetCache *ocipackage.OciLayoutPackage
				assetPath          string
			)

			it.Before(func() {
				var err error
				assetPath = filepath.Join("testdata", "fake-asset-cache.tar")
				expectedAssetCache, err = ocipackage.NewOCILayoutPackage(blob2.NewBlob(
					assetPath, blob2.RawOption),
				)
				h.AssertNil(t, err)
			})
			when("fetching with absolute path", func() {
				it("succeeds", func() {
					absAssetPath, err := filepath.Abs("testdata")
					h.AssertNil(t, err)

					mockFileFetcher.EXPECT().FetchFileAssets(gomock.Any(), gomock.Any(), absAssetPath).
						Return([]*ocipackage.OciLayoutPackage{expectedAssetCache}, nil)

					actualAssets, err := subject.FetchAssets([]string{absAssetPath})
					h.AssertNil(t, err)

					h.AssertEq(t, len(actualAssets), 1)

					assertSameAssetLayers(t, actualAssets[0], expectedAssetCache)
				})
			})
			when("fetching using local path", func() {
				it("Uses the current working directory as default", func() {
					cwd, err := os.Getwd()
					h.AssertNil(t, err)

					mockFileFetcher.EXPECT().FetchFileAssets(gomock.Any(), cwd, assetPath).
						Return([]*ocipackage.OciLayoutPackage{expectedAssetCache}, nil)

					actualAssets, err := subject.FetchAssets([]string{assetPath}, asset.WithWorkingDir(cwd))
					h.AssertNil(t, err)

					h.AssertEq(t, len(actualAssets), 1)

					assertSameAssetLayers(t, actualAssets[0], expectedAssetCache)
				})
				when("WorkingDir option", func() {
					it("uses passed path as the working directory and checks for assets", func() {
						absAssetPath, err := filepath.Abs(assetPath)
						h.AssertNil(t, err)

						fileName := filepath.Base(absAssetPath)
						otherWorkingDir := filepath.Dir(absAssetPath)
						mockFileFetcher.EXPECT().FetchFileAssets(gomock.Any(), otherWorkingDir, fileName).
							Return([]*ocipackage.OciLayoutPackage{expectedAssetCache}, nil)

						actualAssets, err := subject.FetchAssets([]string{fileName}, asset.WithWorkingDir(otherWorkingDir))
						h.AssertNil(t, err)

						h.AssertEq(t, len(actualAssets), 1)

						assertSameAssetLayers(t, actualAssets[0], expectedAssetCache)
					})
				})
			})
		})

		when("fetching uri assets", func() {
			var (
				assetPath          string
				expectedAssetCache *ocipackage.OciLayoutPackage
			)
			it.Before(func() {
				var err error
				assetPath = filepath.Join("testdata", "fake-asset-cache.tar")
				expectedAssetCache, err = ocipackage.NewOCILayoutPackage(blob2.NewBlob(
					assetPath, blob2.RawOption),
				)
				h.AssertNil(t, err)
			})
			it("uses the URI fetcher for all schemes", func() {
				assetName := "scheme:///some/asset"
				mockURIFetcher.EXPECT().FetchURIAssets(gomock.Any(), assetName).
					Return([]*ocipackage.OciLayoutPackage{expectedAssetCache}, nil)

				actualAssets, err := subject.FetchAssets([]string{assetName})
				h.AssertNil(t, err)

				h.AssertEq(t, len(actualAssets), 1)

				assertSameAssetLayers(t, actualAssets[0], expectedAssetCache)
			})
		})
	})

	when("failure cases", func() {
		when("unable to determine locator type from asset name", func() {
			it("errors with helpful message", func() {
				assetName := ":::"
				_, err := subject.FetchAssets([]string{assetName})
				h.AssertError(t, err, "unable to determine asset type from name: :::")
			})
		})
		when("unable to fetch asset", func() {
			it("errors with helpful message", func() {
				assetName := "scheme:///some/asset"
				mockURIFetcher.EXPECT().FetchURIAssets(gomock.Any(), assetName).
					Return([]*ocipackage.OciLayoutPackage{}, errors.New("bad bad error"))

				_, err := subject.FetchAssets([]string{assetName})
				h.AssertError(t, err, `unable to fetch asset of type "URILocator": bad bad error`)
			})
		})
	})
}

func newFakeAssetImage(layerContents, tmpDir string) (*fakes.Image, error) {
	result := fakes.NewImage("fake-image", "", nil)
	aMap := dist.AssetMap{
		"some-sha256": dist.AssetValue{
			ID:          "some-asset",
			Version:     "1.2.3",
			Name:        "default fake asset",
			LayerDiffID: "fake-layer-diffID",
			Stacks:      []string{"some-stack", "some-other-stack"},
		},
	}
	aMapJSON, err := json.Marshal(aMap)
	if err != nil {
		return nil, err
	}

	err = result.SetLabel("io.buildpacks.asset.layers", string(aMapJSON))
	if err != nil {
		return nil, err
	}

	fakeLayer := filepath.Join(tmpDir, layerContents)
	if err := ioutil.WriteFile(fakeLayer, []byte(layerContents), os.ModePerm); err != nil {
		return nil, err
	}

	err = result.AddLayerWithDiffID(fakeLayer, "fake-layer-diffID")
	if err != nil {
		return nil, err
	}

	return result, nil
}
