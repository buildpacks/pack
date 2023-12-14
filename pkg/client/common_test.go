package client

import (
	"bytes"
	"testing"

	"github.com/heroku/color"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	"github.com/buildpacks/pack/internal/builder"
	ifakes "github.com/buildpacks/pack/internal/fakes"
	"github.com/buildpacks/pack/pkg/logging"
	h "github.com/buildpacks/pack/testhelpers"
)

func TestCommon(t *testing.T) {
	color.Disable(true)
	defer color.Disable(false)
	spec.Run(t, "build", testCommon, spec.Report(report.Terminal{}))
}

func testCommon(t *testing.T, when spec.G, it spec.S) {
	when("#resolveRunImage", func() {
		var (
			subject         *Client
			outBuf          bytes.Buffer
			logger          logging.Logger
			runImageName    string
			defaultRegistry string
			defaultMirror   string
			gcrRegistry     string
			gcrRunMirror    string
			stackInfo       builder.StackMetadata
			accessChecker   *ifakes.FakeAccessChecker
			assert          = h.NewAssertionManager(t)
		)

		it.Before(func() {
			logger = logging.NewLogWithWriters(&outBuf, &outBuf)

			var err error
			subject, err = NewClient(WithLogger(logger))
			assert.Nil(err)

			defaultRegistry = "default.registry.io"
			runImageName = "stack/run"
			defaultMirror = defaultRegistry + "/" + runImageName
			gcrRegistry = "gcr.io"
			gcrRunMirror = gcrRegistry + "/" + runImageName
			stackInfo = builder.StackMetadata{
				RunImage: builder.RunImageMetadata{
					Image: runImageName,
					Mirrors: []string{
						defaultMirror, gcrRunMirror,
					},
				},
			}
			accessChecker = ifakes.NewFakeAccessChecker()
		})

		when("passed specific run image", func() {
			it("selects that run image", func() {
				runImgFlag := "flag/passed-run-image"
				runImageName := subject.resolveRunImage(runImgFlag, defaultRegistry, "", stackInfo.RunImage, nil, false, accessChecker)
				assert.Equal(runImageName, runImgFlag)
			})
		})

		when("publish is true", func() {
			it("defaults to run-image in registry publishing to", func() {
				runImageName := subject.resolveRunImage("", gcrRegistry, defaultRegistry, stackInfo.RunImage, nil, true, accessChecker)
				assert.Equal(runImageName, gcrRunMirror)
			})

			it("prefers config defined run image mirror to stack defined run image mirror", func() {
				configMirrors := map[string][]string{
					runImageName: {defaultRegistry + "/unique-run-img"},
				}
				runImageName := subject.resolveRunImage("", defaultRegistry, "", stackInfo.RunImage, configMirrors, true, accessChecker)
				assert.NotEqual(runImageName, defaultMirror)
				assert.Equal(runImageName, defaultRegistry+"/unique-run-img")
			})

			it("returns a config mirror if no match to target registry", func() {
				configMirrors := map[string][]string{
					runImageName: {defaultRegistry + "/unique-run-img"},
				}
				runImageName := subject.resolveRunImage("", "test.registry.io", "", stackInfo.RunImage, configMirrors, true, accessChecker)
				assert.NotEqual(runImageName, defaultMirror)
				assert.Equal(runImageName, defaultRegistry+"/unique-run-img")
			})
		})

		// If publish is false, we are using the local daemon, and want to match to the builder registry
		when("publish is false", func() {
			it("defaults to run-image in registry publishing to", func() {
				runImageName := subject.resolveRunImage("", gcrRegistry, defaultRegistry, stackInfo.RunImage, nil, false, accessChecker)
				assert.Equal(runImageName, defaultMirror)
				assert.NotEqual(runImageName, gcrRunMirror)
			})

			it("prefers config defined run image mirror to stack defined run image mirror", func() {
				configMirrors := map[string][]string{
					runImageName: {defaultRegistry + "/unique-run-img"},
				}
				runImageName := subject.resolveRunImage("", gcrRegistry, defaultRegistry, stackInfo.RunImage, configMirrors, false, accessChecker)
				assert.NotEqual(runImageName, defaultMirror)
				assert.Equal(runImageName, defaultRegistry+"/unique-run-img")
			})

			it("returns a config mirror if no match to target registry", func() {
				configMirrors := map[string][]string{
					runImageName: {defaultRegistry + "/unique-run-img"},
				}
				runImageName := subject.resolveRunImage("", defaultRegistry, "test.registry.io", stackInfo.RunImage, configMirrors, false, accessChecker)
				assert.NotEqual(runImageName, defaultMirror)
				assert.Equal(runImageName, defaultRegistry+"/unique-run-img")
			})
		})

		when("desirable run-image is not accessible", func() {
			it.Before(func() {
				accessChecker.RegistriesToFail = []string{
					gcrRunMirror,
					stackInfo.RunImage.Image,
				}
			})

			it.After(func() {
				accessChecker.RegistriesToFail = nil
			})

			it("selects the first accessible run-image", func() {
				runImageName := subject.resolveRunImage("", gcrRegistry, defaultRegistry, stackInfo.RunImage, nil, true, accessChecker)
				assert.Equal(runImageName, defaultMirror)
			})
		})
	})
}
