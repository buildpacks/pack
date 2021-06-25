package blob_test

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/pkg/errors"
	"github.com/sclevine/spec"

	"github.com/buildpacks/pack/internal/blob"
	"github.com/buildpacks/pack/testhelpers"
	testmocks2 "github.com/buildpacks/pack/testmocks"
)

func TestDownloadManager(t *testing.T) {
	spec.Run(t, "DownloadManager", testDownloadManager)
}

func testDownloadManager(t *testing.T, when spec.G, it spec.S) {
	var (
		assert         = testhelpers.NewAssertionManager(t)
		workerCount    = 2
		mockController *gomock.Controller
		mockDownloader *testmocks2.MockDownloader
		tmpDir         string
	)
	it.Before(func() {
		mockController = gomock.NewController(t)
		mockDownloader = testmocks2.NewMockDownloader(mockController)

		var err error
		tmpDir, err = ioutil.TempDir("", "downloadmanager-test")
		assert.Nil(err)
	})
	when("#DownloadAndValidation", func() {
		var (
			firstBlob  blob.Blob
			secondBlob blob.Blob
			thirdBlob  blob.Blob
		)
		it.Before(func() {
			for _, file := range []string{"first-asset", "second-asset", "third-asset"} {
				contents := []byte(fmt.Sprintf("%s-contents", file))
				assert.Succeeds(ioutil.WriteFile(filepath.Join(tmpDir, file), contents, os.ModePerm))
			}

			firstBlob = blob.NewBlob(filepath.Join(tmpDir, "first-asset"))
			secondBlob = blob.NewBlob(filepath.Join(tmpDir, "second-asset"))
			thirdBlob = blob.NewBlob(filepath.Join(tmpDir, "third-asset"))
		})

		it("downloads and validates", func() {
			subject := blob.NewDownloadManager(mockDownloader, workerCount)

			mockDownloader.EXPECT().Download(gomock.Any(), "https://first-asset", gomock.Any()).Return(firstBlob, nil)
			mockDownloader.EXPECT().Download(gomock.Any(), "https://second-asset", gomock.Any()).Return(secondBlob, nil)
			mockDownloader.EXPECT().Download(gomock.Any(), "https://third-asset", gomock.Any()).Return(thirdBlob, nil)
			mockDownloader.EXPECT().Download(gomock.Any(), "https://first-asset-duplicate", gomock.Any()).Return(firstBlob, nil)

			firstJob := blob.DownloadJob{URI: "https://first-asset", Sha256: "first-asset-sha256"}
			secondJob := blob.DownloadJob{URI: "https://second-asset", Sha256: "second-asset-sha256"}
			thirdJob := blob.DownloadJob{URI: "https://third-asset", Sha256: "third-asset-sha256"}
			firstJobDup := blob.DownloadJob{URI: "https://first-asset-duplicate", Sha256: "first-asset-sha256"}
			jobs := []blob.DownloadJob{firstJob, secondJob, thirdJob, firstJobDup}

			results, err := subject.DownloadAndValidate(context.Background(), jobs...)
			assert.Nil(err)

			assert.Equal(len(results), 4)

			containsDownloadJobKeys(t, results, jobs)

			containsExpectedBlob(t, results[firstJob], "first-asset-contents")
			containsExpectedBlob(t, results[secondJob], "second-asset-contents")
			containsExpectedBlob(t, results[thirdJob], "third-asset-contents")
			containsExpectedBlob(t, results[firstJobDup], "first-asset-contents")
		})

		when("failure cases", func() {
			when("a download fails", func() {
				it("returns an error message", func() {
					subject := blob.NewDownloadManager(mockDownloader, workerCount)
					mockDownloader.EXPECT().Download(gomock.Any(), "https://first-asset", gomock.Any()).Return(nil, errors.New("download error"))

					_, err := subject.DownloadAndValidate(context.Background(), blob.DownloadJob{URI: "https://first-asset", Sha256: "first-asset-sha256"})
					assert.ErrorContains(err, `the following errors occurred during download: "download error"`)
				})
			})
		})
	})
}

func containsDownloadJobKeys(t *testing.T, actual map[blob.DownloadJob]blob.DownloadResult, expectedKeys []blob.DownloadJob) {
	t.Helper()

	uniqueKeys := map[blob.DownloadJob]interface{}{}
	for _, key := range expectedKeys {
		_, ok := actual[key]
		if !ok {
			t.Fatalf("unable to find %#v in map", key)
		}
		uniqueKeys[key] = true
	}

	if len(uniqueKeys) < len(actual) {
		t.Fatalf("actual map has extra keys")
	}
}

func containsExpectedBlob(t *testing.T, result blob.DownloadResult, expectedContents string) {
	t.Helper()
	assert := testhelpers.NewAssertionManager(t)

	r, err := result.Blob.Open()
	assert.Nil(err)

	buf := bytes.NewBuffer(nil)
	_, err = io.Copy(buf, r)
	assert.Nil(err)

	assert.Contains(buf.String(), expectedContents)
}
