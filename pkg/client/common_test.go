package client

import (
	"bytes"
	"testing"

	"github.com/buildpacks/lifecycle/auth"
	"github.com/golang/mock/gomock"
	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/heroku/color"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	"github.com/buildpacks/pack/internal/builder"
	"github.com/buildpacks/pack/pkg/image"
	"github.com/buildpacks/pack/pkg/logging"
	"github.com/buildpacks/pack/pkg/testmocks"
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
			keychain        authn.Keychain
			runImageName    string
			defaultRegistry string
			defaultMirror   string
			gcrRegistry     string
			gcrRunMirror    string
			stackInfo       builder.StackMetadata
			assert          = h.NewAssertionManager(t)
			publish         bool
			err             error
		)

		it.Before(func() {
			logger = logging.NewLogWithWriters(&outBuf, &outBuf)

			keychain, err = auth.DefaultKeychain("pack-test/dummy")
			h.AssertNil(t, err)

			subject, err = NewClient(WithLogger(logger), WithKeychain(keychain))
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
			it.Before(func() {
				publish = false
			})

			it("selects that run image", func() {
				runImgFlag := "flag/passed-run-image"
				runImageName = subject.resolveRunImage(runImgFlag, defaultRegistry, "", stackInfo.RunImage, nil, publish)
				assert.Equal(runImageName, runImgFlag)
			})
		})

		when("publish is true", func() {
			when("desirable run-image are accessible", func() {
				it.Before(func() {
					publish = true
					mockFetcher := fetcherWithCheckReadAccess(t, publish, func(repo string) bool {
						return true
					})
					subject, err = NewClient(WithLogger(logger), WithKeychain(keychain), WithFetcher(mockFetcher))
					h.AssertNil(t, err)
				})

				it("defaults to run-image in registry publishing to", func() {
					runImageName = subject.resolveRunImage("", gcrRegistry, defaultRegistry, stackInfo.RunImage, nil, publish)
					assert.Equal(runImageName, gcrRunMirror)
				})

				it("prefers config defined run image mirror to stack defined run image mirror", func() {
					configMirrors := map[string][]string{
						runImageName: {defaultRegistry + "/unique-run-img"},
					}
					runImageName = subject.resolveRunImage("", defaultRegistry, "", stackInfo.RunImage, configMirrors, publish)
					assert.NotEqual(runImageName, defaultMirror)
					assert.Equal(runImageName, defaultRegistry+"/unique-run-img")
				})

				it("returns a config mirror if no match to target registry", func() {
					configMirrors := map[string][]string{
						runImageName: {defaultRegistry + "/unique-run-img"},
					}
					runImageName = subject.resolveRunImage("", "test.registry.io", "", stackInfo.RunImage, configMirrors, publish)
					assert.NotEqual(runImageName, defaultMirror)
					assert.Equal(runImageName, defaultRegistry+"/unique-run-img")
				})
			})

			when("desirable run-images are not accessible", func() {
				it.Before(func() {
					publish = true
					mockFetcher := fetcherWithCheckReadAccess(t, publish, func(repo string) bool {
						if repo == gcrRunMirror || repo == stackInfo.RunImage.Image {
							return false
						}
						return true
					})
					subject, err = NewClient(WithLogger(logger), WithKeychain(keychain), WithFetcher(mockFetcher))
					h.AssertNil(t, err)
				})

				it("selects the first accessible run-image", func() {
					runImageName = subject.resolveRunImage("", gcrRegistry, defaultRegistry, stackInfo.RunImage, nil, publish)
					assert.Equal(runImageName, defaultMirror)
				})
			})
		})

		// If publish is false, we are using the local daemon, and want to match to the builder registry
		when("publish is false", func() {
			it.Before(func() {
				publish = false
			})

			it("defaults to run-image in registry publishing to", func() {
				runImageName = subject.resolveRunImage("", gcrRegistry, defaultRegistry, stackInfo.RunImage, nil, publish)
				assert.Equal(runImageName, defaultMirror)
				assert.NotEqual(runImageName, gcrRunMirror)
			})

			it("prefers config defined run image mirror to stack defined run image mirror", func() {
				configMirrors := map[string][]string{
					runImageName: {defaultRegistry + "/unique-run-img"},
				}
				runImageName = subject.resolveRunImage("", gcrRegistry, defaultRegistry, stackInfo.RunImage, configMirrors, publish)
				assert.NotEqual(runImageName, defaultMirror)
				assert.Equal(runImageName, defaultRegistry+"/unique-run-img")
			})

			it("returns a config mirror if no match to target registry", func() {
				configMirrors := map[string][]string{
					runImageName: {defaultRegistry + "/unique-run-img"},
				}
				runImageName = subject.resolveRunImage("", defaultRegistry, "test.registry.io", stackInfo.RunImage, configMirrors, publish)
				assert.NotEqual(runImageName, defaultMirror)
				assert.Equal(runImageName, defaultRegistry+"/unique-run-img")
			})

			when("desirable run-image are empty", func() {
				it.Before(func() {
					stackInfo = builder.StackMetadata{
						RunImage: builder.RunImageMetadata{
							Image: "stack/run-image",
						},
					}
				})

				it("selects the builder run-image", func() {
					// issue: https://github.com/buildpacks/pack/issues/2078
					runImageName = subject.resolveRunImage("", "", "", stackInfo.RunImage, nil, publish)
					assert.Equal(runImageName, "stack/run-image")
				})
			})
		})
	})
}

func fetcherWithCheckReadAccess(t *testing.T, publish bool, checker image.CheckReadAccess) *testmocks.MockImageFetcher {
	mockController := gomock.NewController(t)
	mockFetcher := testmocks.NewMockImageFetcher(mockController)
	mockFetcher.EXPECT().CheckReadAccessValidator(image.FetchOptions{Daemon: !publish}).Return(checker)
	return mockFetcher
}
