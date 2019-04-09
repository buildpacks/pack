package pack_test

import (
	"bytes"
	"context"
	"crypto/md5"
	"fmt"
	"math/rand"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/fatih/color"
	"github.com/golang/mock/gomock"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	"github.com/buildpack/pack"
	"github.com/buildpack/pack/config"
	"github.com/buildpack/pack/logging"
	"github.com/buildpack/pack/mocks"
	h "github.com/buildpack/pack/testhelpers"
)

func TestRun(t *testing.T) {
	color.NoColor = true
	rand.Seed(time.Now().UTC().UnixNano())
	spec.Run(t, "run", testRun, spec.Parallel(), spec.Report(report.Terminal{}))
}

func testRun(t *testing.T, when spec.G, it spec.S) {
	var (
		outBuf         bytes.Buffer
		errBuf         bytes.Buffer
		logger         *logging.Logger
		mockController *gomock.Controller
	)

	it.Before(func() {
		mockController = gomock.NewController(t)
		logger = logging.NewLogger(&outBuf, &errBuf, true, false)
	})

	it.After(func() {
		mockController.Finish()
	})

	when("#RunConfigFromFlags", func() {
		var (
			mockController   *gomock.Controller
			factory          *pack.BuildFactory
			MockImageFetcher *mocks.MockImageFetcher
			mockCache        *mocks.MockCache
		)

		it.Before(func() {
			mockController = gomock.NewController(t)
			MockImageFetcher = mocks.NewMockImageFetcher(mockController)
			mockCache = mocks.NewMockCache(mockController)
			factory = &pack.BuildFactory{
				Logger:  logger,
				Fetcher: MockImageFetcher,
				Cache:   mockCache,
				Config:  &config.Config{},
			}

			mockCache.EXPECT().Image().Return("some-volume").AnyTimes()
		})

		it.After(func() {
			mockController.Finish()
		})

		it("creates args RunConfig derived from args BuildConfig", func() {
			mockBuilderImage := mocks.NewMockImage(mockController)
			MockImageFetcher.EXPECT().Fetch(gomock.Any(), "some/builder", true, true).Return(mockBuilderImage, nil)

			mockRunImage := mocks.NewMockImage(mockController)
			MockImageFetcher.EXPECT().Fetch(gomock.Any(), "some/run", true, true).Return(mockRunImage, nil)

			run, err := factory.RunConfigFromFlags(context.TODO(), &pack.RunFlags{
				BuildFlags: pack.BuildFlags{
					AppDir:   "acceptance/testdata/node_app",
					Builder:  "some/builder",
					RunImage: "some/run",
				},
				Ports: []string{"1370"},
			})
			h.AssertNil(t, err)

			absAppDir, _ := filepath.Abs("acceptance/testdata/node_app")
			absAppDirMd5 := fmt.Sprintf("pack.local/run/%x", md5.Sum([]byte(absAppDir)))
			h.AssertEq(t, run.RepoName, absAppDirMd5)
			h.AssertEq(t, run.Ports, []string{"1370"})

			build, ok := run.Build.(*pack.BuildConfig)
			h.AssertEq(t, ok, true)
			for _, field := range []string{
				"RepoName",
				"Logger",
			} {
				h.AssertSameInstance(
					t,
					reflect.Indirect(reflect.ValueOf(run)).FieldByName(field).Interface(),
					reflect.Indirect(reflect.ValueOf(build)).FieldByName(field).Interface(),
				)
			}
		})

	})
}
