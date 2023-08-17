package build_test

import (
	"bytes"
	"context"
	"io"

	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/apex/log"

	"github.com/buildpacks/pack/internal/build"
	"github.com/buildpacks/pack/internal/build/fakes"

	"github.com/docker/docker/api/types"
	"github.com/golang/mock/gomock"
	"github.com/heroku/color"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	"github.com/buildpacks/pack/pkg/archive"
	"github.com/buildpacks/pack/pkg/logging"
	h "github.com/buildpacks/pack/testhelpers"

	mockdocker "github.com/buildpacks/pack/internal/build/mockdocker"
)

func TestBuildDockerfiles(t *testing.T) {
	color.Disable(true)
	defer color.Disable(false)
	spec.Run(t, "buildExtendByDocker", testBuildDockerfiles, spec.Report(report.Terminal{}), spec.Sequential())
}

const (
	argUserID  = "user_id"
	argGroupID = "group_id"
)

func testBuildDockerfiles(t *testing.T, when spec.G, it spec.S) {
	var (
		mockDockerClient *mockdocker.MockDockerClient
		mockController   *gomock.Controller
		lifecycle        *build.LifecycleExecution
		tmpDir           string

		// lifecycle options
		providedClearCache            bool
		providedPublish               bool
		providedBuilderImage          = "some-registry.com/some-namespace/some-builder-name"
		extendedBuilderImage          = "some-registry.com/some-namespace/some-builder-name-extended"
		configureDefaultTestLifecycle func(opts *build.LifecycleOptions)
		lifecycleOps                  []func(*build.LifecycleOptions)
	)

	it.Before(func() {
		var err error
		mockController = gomock.NewController(t)
		mockDockerClient = mockdocker.NewMockDockerClient(mockController)
		h.AssertNil(t, err)

		configureDefaultTestLifecycle = func(opts *build.LifecycleOptions) {
			opts.BuilderImage = providedBuilderImage
			opts.ClearCache = providedClearCache
			opts.Publish = providedPublish
		}

		lifecycleOps = []func(*build.LifecycleOptions){configureDefaultTestLifecycle}
	})

	when("Extend Build Image By Docker", func() {
		it("should extend build image using 1 extension", func() {
			// set tmp directory
			tmpDir = "./testdata/fake-tmp/build-extension/single"
			lifecycle = getTestLifecycleExec(t, true, tmpDir, mockDockerClient, lifecycleOps...)
			expectedBuilder := lifecycle.Builder()
			expectedBuildContext := archive.ReadDirAsTar(filepath.Dir(filepath.Join(tmpDir, "fake-tmp", "build-extension", "single", "build", "samples_test", "Dockerfile")), "/", 0, 0, -1, true, false, func(file string) bool { return true })
			// Set up expected Build Args
			UID := strconv.Itoa(expectedBuilder.UID())
			GID := strconv.Itoa(expectedBuilder.GID())
			expectedbuildArguments := map[string]*string{}
			expectedbuildArguments["base_image"] = &providedBuilderImage
			expectedbuildArguments[argUserID] = &UID
			expectedbuildArguments[argGroupID] = &GID
			expectedBuildOptions := types.ImageBuildOptions{
				Context:    expectedBuildContext,
				Dockerfile: "Dockerfile",
				Tags:       []string{extendedBuilderImage},
				Remove:     true,
				BuildArgs:  expectedbuildArguments,
			}
			mockResponse := types.ImageBuildResponse{
				Body:   io.NopCloser(strings.NewReader("mock-build-response-body")),
				OSType: "linux",
			}
			mockDockerClient.EXPECT().ImageBuild(gomock.Any(), gomock.Any(), gomock.Any()).Do(func(_ context.Context, buildContext io.ReadCloser, buildOptions types.ImageBuildOptions) {
				compBuildOptions(t, expectedBuildOptions, buildOptions)
			}).Return(mockResponse, nil).Times(1)
			err := lifecycle.ExtendBuildByDaemon(context.Background())
			h.AssertNil(t, err)
		})

		it("should extend build image using multiple extension", func() {
			// set tmp directory
			tmpDir = "./testdata/fake-tmp/build-extension/multi"
			lifecycle = getTestLifecycleExec(t, true, tmpDir, mockDockerClient, lifecycleOps...)
			mockResponse := types.ImageBuildResponse{
				Body:   io.NopCloser(strings.NewReader("mock-build-response-body")),
				OSType: "linux",
			}
			mockDockerClient.EXPECT().ImageBuild(gomock.Any(), gomock.Any(), gomock.Any()).Return(mockResponse, nil).Times(2)
			err := lifecycle.ExtendBuildByDaemon(context.Background())
			h.AssertNil(t, err)
		})
	})
}
func GetTestLifecycleExecErr(t *testing.T, logVerbose bool, tmpDir string, mockDockerClient *mockdocker.MockDockerClient, ops ...func(*build.LifecycleOptions)) (*build.LifecycleExecution, error) {
	var outBuf bytes.Buffer
	logger := logging.NewLogWithWriters(&outBuf, &outBuf)
	if logVerbose {
		logger.Level = log.DebugLevel
	}
	defaultBuilder, err := fakes.NewFakeBuilder()
	h.AssertNil(t, err)

	opts := build.LifecycleOptions{
		AppPath:    "some-app-path",
		Builder:    defaultBuilder,
		HTTPProxy:  "some-http-proxy",
		HTTPSProxy: "some-https-proxy",
		NoProxy:    "some-no-proxy",
		Termui:     &fakes.FakeTermui{},
	}

	for _, op := range ops {
		op(&opts)
	}

	return build.NewLifecycleExecution(logger, mockDockerClient, tmpDir, opts)
}

func getTestLifecycleExec(t *testing.T, logVerbose bool, tmpDir string, mockDockerClient *mockdocker.MockDockerClient, ops ...func(*build.LifecycleOptions)) *build.LifecycleExecution {
	t.Helper()

	lifecycleExec, err := GetTestLifecycleExecErr(t, logVerbose, tmpDir, mockDockerClient, ops...)
	h.AssertNil(t, err)
	return lifecycleExec
}

func compBuildOptions(t *testing.T, expectedBuildOptions types.ImageBuildOptions, actualBuildOptions types.ImageBuildOptions) {
	t.Helper()
	h.AssertEq(t, expectedBuildOptions.Dockerfile, actualBuildOptions.Dockerfile)
	h.AssertEq(t, expectedBuildOptions.Tags, actualBuildOptions.Tags)
	h.AssertEq(t, expectedBuildOptions.Remove, actualBuildOptions.Remove)
	h.AssertEq(t, expectedBuildOptions.BuildArgs["base_image"], actualBuildOptions.BuildArgs["base_image"])
	h.AssertEq(t, expectedBuildOptions.BuildArgs[argUserID], actualBuildOptions.BuildArgs[argUserID])
	h.AssertEq(t, expectedBuildOptions.BuildArgs[argGroupID], actualBuildOptions.BuildArgs[argGroupID])
}
