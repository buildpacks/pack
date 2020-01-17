package builder_test

import (
	"io/ioutil"
	"path/filepath"
	"testing"

	"github.com/Masterminds/semver"

	"github.com/buildpacks/imgutil/fakes"
	"github.com/golang/mock/gomock"
	"github.com/heroku/color"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	"github.com/buildpacks/pack/internal/api"
	"github.com/buildpacks/pack/internal/archive"
	"github.com/buildpacks/pack/internal/builder"
	"github.com/buildpacks/pack/internal/builder/testmocks"
	"github.com/buildpacks/pack/logging"
	h "github.com/buildpacks/pack/testhelpers"
)

func TestCompat(t *testing.T) {
	color.Disable(true)
	defer color.Disable(false)
	spec.Run(t, "Compat", testCompat, spec.Parallel(), spec.Report(report.Terminal{}))
}

func testCompat(t *testing.T, when spec.G, it spec.S) {
	var (
		baseImage      *fakes.Image
		subject        *builder.Builder
		mockController *gomock.Controller
		mockLifecycle  *testmocks.MockLifecycle
		logger         logging.Logger
	)

	it.Before(func() {
		baseImage = fakes.NewImage("base/image", "", nil)
		mockController = gomock.NewController(t)
		mockLifecycle = testmocks.NewMockLifecycle(mockController)

		h.AssertNil(t, baseImage.SetEnv("CNB_USER_ID", "1234"))
		h.AssertNil(t, baseImage.SetEnv("CNB_GROUP_ID", "4321"))
		h.AssertNil(t, baseImage.SetLabel("io.buildpacks.stack.id", "some.stack.id"))

		logger = logging.New(ioutil.Discard)

		var err error
		subject, err = builder.New(baseImage, "some/builder")
		h.AssertNil(t, err)
	})

	it.After(func() {
		baseImage.Cleanup()
		mockController.Finish()
	})

	it("should create a compat lifecycle symlink", func() {
		mockLifecycle.EXPECT().Open().Return(archive.ReadDirAsTar(
			filepath.Join("testdata", "lifecycle"), ".", 0, 0, -1), nil).AnyTimes()
		mockLifecycle.EXPECT().Descriptor().Return(builder.LifecycleDescriptor{
			Info: builder.LifecycleInfo{
				Version: &builder.Version{Version: *semver.MustParse("0.4.0")},
			},
			API: builder.LifecycleAPI{
				PlatformVersion:  api.MustParse("0.2"),
				BuildpackVersion: api.MustParse("0.2"),
			},
		}).AnyTimes()

		h.AssertNil(t, subject.SetLifecycle(mockLifecycle))
		h.AssertNil(t, subject.Save(logger))
		h.AssertEq(t, baseImage.IsSaved(), true)

		layerTar, err := baseImage.FindLayerWithPath("/lifecycle")
		h.AssertNil(t, err)
		h.AssertOnTarEntry(t, layerTar, "/lifecycle",
			h.SymlinksTo("/cnb/lifecycle"),
			h.HasModTime(archive.NormalizedDateTime),
		)
	})
}
