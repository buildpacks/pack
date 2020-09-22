package pack

import (
	"bytes"
	"testing"

	"github.com/heroku/color"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	"github.com/buildpacks/pack/internal/builder"
	ilogging "github.com/buildpacks/pack/internal/logging"
	"github.com/buildpacks/pack/logging"
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
			assert          = h.NewAssertionManager(t)
		)

		it.Before(func() {
			logger = ilogging.NewLogWithWriters(&outBuf, &outBuf)

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
		})

		when("passed specific run image", func() {
			it("selects that run image", func() {
				runImgFlag := "flag/passed-run-image"
				runImageName := subject.resolveRunImage(runImgFlag, defaultRegistry, "", stackInfo, nil, false)
				assert.Equal(runImageName, runImgFlag)
			})
		})

		when("publish is true", func() {
			it("defaults to run-image in registry publishing to", func() {
				runImageName := subject.resolveRunImage("", gcrRegistry, defaultRegistry, stackInfo, nil, true)
				assert.Equal(runImageName, gcrRunMirror)
			})

			it("prefers config defined run image mirror to stack defined run image mirror", func() {
				configMirrors := map[string][]string{
					runImageName: []string{defaultRegistry + "/unique-run-img"},
				}
				runImageName := subject.resolveRunImage("", defaultRegistry, "", stackInfo, configMirrors, true)
				assert.NotEqual(runImageName, defaultMirror)
				assert.Equal(runImageName, defaultRegistry+"/unique-run-img")
			})

			it("returns a config mirror if no match to target registry", func() {
				configMirrors := map[string][]string{
					runImageName: []string{defaultRegistry + "/unique-run-img"},
				}
				runImageName := subject.resolveRunImage("", "test.registry.io", "", stackInfo, configMirrors, true)
				assert.NotEqual(runImageName, defaultMirror)
				assert.Equal(runImageName, defaultRegistry+"/unique-run-img")
			})
		})

		// If publish is false, we are using the local daemon, and want to match to the builder registry
		when("publish is false", func() {
			it("defaults to run-image in registry publishing to", func() {
				runImageName := subject.resolveRunImage("", gcrRegistry, defaultRegistry, stackInfo, nil, false)
				assert.Equal(runImageName, defaultMirror)
				assert.NotEqual(runImageName, gcrRunMirror)
			})

			it("prefers config defined run image mirror to stack defined run image mirror", func() {
				configMirrors := map[string][]string{
					runImageName: []string{defaultRegistry + "/unique-run-img"},
				}
				runImageName := subject.resolveRunImage("", gcrRegistry, defaultRegistry, stackInfo, configMirrors, false)
				assert.NotEqual(runImageName, defaultMirror)
				assert.Equal(runImageName, defaultRegistry+"/unique-run-img")
			})

			it("returns a config mirror if no match to target registry", func() {
				configMirrors := map[string][]string{
					runImageName: []string{defaultRegistry + "/unique-run-img"},
				}
				runImageName := subject.resolveRunImage("", defaultRegistry, "test.registry.io", stackInfo, configMirrors, false)
				assert.NotEqual(runImageName, defaultMirror)
				assert.Equal(runImageName, defaultRegistry+"/unique-run-img")
			})
		})
	})
}
