package commands_test

import (
	"bytes"
	"fmt"
	"testing"

	"github.com/buildpacks/lifecycle/api"
	"github.com/golang/mock/gomock"
	"github.com/heroku/color"
	"github.com/pkg/errors"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"
	"github.com/spf13/cobra"

	"github.com/buildpacks/pack/internal/commands"
	"github.com/buildpacks/pack/internal/commands/testmocks"
	"github.com/buildpacks/pack/internal/config"
	"github.com/buildpacks/pack/pkg/buildpack"
	"github.com/buildpacks/pack/pkg/client"
	"github.com/buildpacks/pack/pkg/dist"
	"github.com/buildpacks/pack/pkg/image"
	"github.com/buildpacks/pack/pkg/logging"
	h "github.com/buildpacks/pack/testhelpers"
)

const simpleExtensionOutputSection = `Extensions:
  ID                                NAME        VERSION        HOMEPAGE
  some/single-extension             some        0.0.1          single-extension-homepage`

const inspectExtensionOutputTemplate = `Inspecting extension: '%s'

%s

%s
`

const depthExtensionOutputSection = `
Detection Order:
 └ Group #1:
    └ some/top-extension@0.0.1
       ├ Group #1:
       │  ├ some/first-inner-extension@1.0.0
       │  └ some/second-inner-extension@2.0.0
       └ Group #2:
          └ some/first-inner-extension@1.0.0`

const simpleExtensionMixinOutputSection = `
  ID: io.extensions.stacks.first-stack
    Mixins:
      mixin1
      mixin2
      build:mixin3
      build:mixin4
  ID: io.extensions.stacks.second-stack
    Mixins:
      mixin1
      mixin2`

func TestExtensionInspectCommand(t *testing.T) {
	color.Disable(true)
	defer color.Disable(false)
	spec.Run(t, "ExtensionInspectCommand", testExtensionInspectCommand, spec.Sequential(), spec.Report(report.Terminal{}))
}

func testExtensionInspectCommand(t *testing.T, when spec.G, it spec.S) {
	apiVersion, err := api.NewVersion("0.2")
	if err != nil {
		t.Fail()
	}

	var (
		command        *cobra.Command
		logger         logging.Logger
		outBuf         bytes.Buffer
		mockController *gomock.Controller
		mockClient     *testmocks.MockPackClient
		cfg            config.Config
		complexInfo    *client.ExtensionInfo
		simpleInfo     *client.ExtensionInfo
		assert         = h.NewAssertionManager(t)
	)

	it.Before(func() {
		mockController = gomock.NewController(t)
		mockClient = testmocks.NewMockPackClient(mockController)
		logger = logging.NewLogWithWriters(&outBuf, &outBuf)

		cfg = config.Config{
			DefaultRegistryName: "default-registry",
		}

		complexInfo = &client.ExtensionInfo{
			ExtensionMetadata: buildpack.Metadata{
				ModuleInfo: dist.ModuleInfo{
					ID:       "some/top-extension",
					Version:  "0.0.1",
					Homepage: "top-extension-homepage",
					Name:     "top",
				},
				Stacks: []dist.Stack{
					{ID: "io.extensions.stacks.first-stack", Mixins: []string{"mixin1", "mixin2", "build:mixin3", "build:mixin4"}},
					{ID: "io.extensions.stacks.second-stack", Mixins: []string{"mixin1", "mixin2"}},
				},
			},
			Extensions: []dist.ModuleInfo{
				{
					ID:       "some/first-inner-extension",
					Version:  "1.0.0",
					Homepage: "first-inner-extension-homepage",
				},
				{
					ID:       "some/second-inner-extension",
					Version:  "2.0.0",
					Homepage: "second-inner-extension-homepage",
				},
				{
					ID:       "some/third-inner-extension",
					Version:  "3.0.0",
					Homepage: "third-inner-extension-homepage",
				},
				{
					ID:       "some/top-extension",
					Version:  "0.0.1",
					Homepage: "top-extension-homepage",
					Name:     "top",
				},
			},
			Order: dist.Order{
				{
					Group: []dist.ModuleRef{
						{
							ModuleInfo: dist.ModuleInfo{
								ID:       "some/top-extension",
								Version:  "0.0.1",
								Homepage: "top-extension-homepage",
								Name:     "top",
							},
							Optional: false,
						},
					},
				},
			},
			ExtensionLayers: dist.ModuleLayers{
				"some/first-inner-extension": {
					"1.0.0": {
						API: apiVersion,
						Stacks: []dist.Stack{
							{ID: "io.extensions.stacks.first-stack", Mixins: []string{"mixin1", "mixin2", "build:mixin3", "build:mixin4"}},
							{ID: "io.extensions.stacks.second-stack", Mixins: []string{"mixin1", "mixin2"}},
						},
						Order: dist.Order{
							{
								Group: []dist.ModuleRef{
									{
										ModuleInfo: dist.ModuleInfo{
											ID:      "some/first-inner-extension",
											Version: "1.0.0",
										},
										Optional: false,
									},
									{
										ModuleInfo: dist.ModuleInfo{
											ID:      "some/third-inner-extension",
											Version: "3.0.0",
										},
										Optional: false,
									},
								},
							},
							{
								Group: []dist.ModuleRef{
									{
										ModuleInfo: dist.ModuleInfo{
											ID:      "some/third-inner-extension",
											Version: "3.0.0",
										},
										Optional: false,
									},
								},
							},
						},
						LayerDiffID: "sha256:first-inner-extension-diff-id",
						Homepage:    "first-inner-extension-homepage",
					},
				},
				"some/second-inner-extension": {
					"2.0.0": {
						API: apiVersion,
						Stacks: []dist.Stack{
							{ID: "io.extensions.stacks.first-stack", Mixins: []string{"mixin1", "mixin2", "build:mixin3", "build:mixin4"}},
							{ID: "io.extensions.stacks.second-stack", Mixins: []string{"mixin1", "mixin2"}},
						},
						LayerDiffID: "sha256:second-inner-extension-diff-id",
						Homepage:    "second-inner-extension-homepage",
					},
				},
				"some/third-inner-extension": {
					"3.0.0": {
						API: apiVersion,
						Stacks: []dist.Stack{
							{ID: "io.extensions.stacks.first-stack", Mixins: []string{"mixin1", "mixin2", "build:mixin3", "build:mixin4"}},
							{ID: "io.extensions.stacks.second-stack", Mixins: []string{"mixin1", "mixin2"}},
						},
						LayerDiffID: "sha256:third-inner-extension-diff-id",
						Homepage:    "third-inner-extension-homepage",
					},
				},
				"some/top-extension": {
					"0.0.1": {
						API: apiVersion,
						Order: dist.Order{
							{
								Group: []dist.ModuleRef{
									{
										ModuleInfo: dist.ModuleInfo{
											ID:      "some/first-inner-extension",
											Version: "1.0.0",
										},
										Optional: false,
									},
									{
										ModuleInfo: dist.ModuleInfo{
											ID:      "some/second-inner-extension",
											Version: "2.0.0",
										},
										Optional: false,
									},
								},
							},
							{
								Group: []dist.ModuleRef{
									{
										ModuleInfo: dist.ModuleInfo{
											ID:      "some/first-inner-extension",
											Version: "1.0.0",
										},
										Optional: false,
									},
								},
							},
						},
						LayerDiffID: "sha256:top-extension-diff-id",
						Homepage:    "top-extension-homepage",
						Name:        "top",
					},
				},
			},
		}

		simpleInfo = &client.ExtensionInfo{
			ExtensionMetadata: buildpack.Metadata{
				ModuleInfo: dist.ModuleInfo{
					ID:       "some/single-extension",
					Version:  "0.0.1",
					Homepage: "single-homepage-homepace",
					Name:     "some",
				},
				Stacks: []dist.Stack{
					{ID: "io.extensions.stacks.first-stack", Mixins: []string{"mixin1", "mixin2", "build:mixin3", "build:mixin4"}},
					{ID: "io.extensions.stacks.second-stack", Mixins: []string{"mixin1", "mixin2"}},
				},
			},
			Extensions: []dist.ModuleInfo{
				{
					ID:       "some/single-extension",
					Version:  "0.0.1",
					Name:     "some",
					Homepage: "single-extension-homepage",
				},
				{
					ID:      "some/extension-no-homepage",
					Version: "0.0.2",
				},
			},
			Order: dist.Order{
				{
					Group: []dist.ModuleRef{
						{
							ModuleInfo: dist.ModuleInfo{
								ID:       "some/single-extension",
								Version:  "0.0.1",
								Homepage: "single-extension-homepage",
							},
							Optional: false,
						},
					},
				},
			},
			ExtensionLayers: dist.ModuleLayers{
				"some/single-extension": {
					"0.0.1": {
						API: apiVersion,
						Stacks: []dist.Stack{
							{ID: "io.extensions.stacks.first-stack", Mixins: []string{"mixin1", "mixin2", "build:mixin3", "build:mixin4"}},
							{ID: "io.extensions.stacks.second-stack", Mixins: []string{"mixin1", "mixin2"}},
						},
						LayerDiffID: "sha256:single-extension-diff-id",
						Homepage:    "single-extension-homepage",
						Name:        "some",
					},
				},
			},
		}

		command = commands.ExtensionInspect(logger, cfg, mockClient)
	})

	when("ExtensionInspect", func() {
		when("inspecting an image", func() {
			when("both remote and local image are present", func() {
				it.Before(func() {
					complexInfo.Location = buildpack.PackageLocator
					simpleInfo.Location = buildpack.PackageLocator

					mockClient.EXPECT().InspectExtension(client.InspectExtensionOptions{
						ExtensionName: "test/extension",
						Daemon:        true,
						Registry:      "default-registry",
					}).Return(complexInfo, nil)

					mockClient.EXPECT().InspectExtension(client.InspectExtensionOptions{
						ExtensionName: "test/extension",
						Daemon:        false,
						Registry:      "default-registry",
					}).Return(simpleInfo, nil)
				})

				it("succeeds", func() {
					command.SetArgs([]string{"test/extension"})
					assert.Nil(command.Execute())

					localOutputSection := fmt.Sprintf(inspectExtensionOutputTemplate,
						"test/extension",
						"LOCAL IMAGE:",
						complexExtensionOutputSection)

					remoteOutputSection := fmt.Sprintf("%s\n\n%s",
						"REMOTE IMAGE:",
						simpleExtensionOutputSection)

					assert.AssertTrimmedContains(outBuf.String(), localOutputSection)
					assert.AssertTrimmedContains(outBuf.String(), remoteOutputSection)
				})
			})

			when("only a local image is present", func() {
				it.Before(func() {
					complexInfo.Location = buildpack.PackageLocator
					simpleInfo.Location = buildpack.PackageLocator

					mockClient.EXPECT().InspectExtension(client.InspectExtensionOptions{
						ExtensionName: "only-local-test/extension",
						Daemon:        true,
						Registry:      "default-registry",
					}).Return(complexInfo, nil)

					mockClient.EXPECT().InspectExtension(client.InspectExtensionOptions{
						ExtensionName: "only-local-test/extension",
						Daemon:        false,
						Registry:      "default-registry",
					}).Return(nil, errors.Wrap(image.ErrNotFound, "remote image not found!"))
				})

				it("displays output for local image", func() {
					command.SetArgs([]string{"only-local-test/extension"})
					assert.Nil(command.Execute())

					expectedOutput := fmt.Sprintf(inspectExtensionOutputTemplate,
						"only-local-test/extension",
						"LOCAL IMAGE:",
						complexExtensionOutputSection)

					assert.AssertTrimmedContains(outBuf.String(), expectedOutput)
				})
			})

			when("only a remote image is present", func() {
				it.Before(func() {
					complexInfo.Location = buildpack.PackageLocator
					simpleInfo.Location = buildpack.PackageLocator

					mockClient.EXPECT().InspectExtension(client.InspectExtensionOptions{
						ExtensionName: "only-remote-test/extension",
						Daemon:        false,
						Registry:      "default-registry",
					}).Return(complexInfo, nil)

					mockClient.EXPECT().InspectExtension(client.InspectExtensionOptions{
						ExtensionName: "only-remote-test/extension",
						Daemon:        true,
						Registry:      "default-registry",
					}).Return(nil, errors.Wrap(image.ErrNotFound, "remote image not found!"))
				})

				it("displays output for remote image", func() {
					command.SetArgs([]string{"only-remote-test/extension"})
					assert.Nil(command.Execute())

					expectedOutput := fmt.Sprintf(inspectExtensionOutputTemplate,
						"only-remote-test/extension",
						"REMOTE IMAGE:",
						complexExtensionOutputSection)

					assert.AssertTrimmedContains(outBuf.String(), expectedOutput)
				})
			})
		})

		when("inspecting a extension uri", func() {
			it.Before(func() {
				simpleInfo.Location = buildpack.URILocator
			})

			when("uri is a local path", func() {
				it.Before(func() {
					mockClient.EXPECT().InspectExtension(client.InspectExtensionOptions{
						ExtensionName: "/path/to/test/extension",
						Daemon:        true,
						Registry:      "default-registry",
					}).Return(simpleInfo, nil)
				})

				it("succeeds", func() {
					command.SetArgs([]string{"/path/to/test/extension"})
					assert.Nil(command.Execute())

					expectedOutput := fmt.Sprintf(inspectExtensionOutputTemplate,
						"/path/to/test/extension",
						"LOCAL ARCHIVE:",
						simpleExtensionOutputSection)

					assert.TrimmedEq(outBuf.String(), expectedOutput)
				})
			})

			when("uri is a http or https location", func() {
				it.Before(func() {
					simpleInfo.Location = buildpack.URILocator
				})
				when("uri is a local path", func() {
					it.Before(func() {
						mockClient.EXPECT().InspectExtension(client.InspectExtensionOptions{
							ExtensionName: "https://path/to/test/extension",
							Daemon:        true,
							Registry:      "default-registry",
						}).Return(simpleInfo, nil)
					})

					it("succeeds", func() {
						command.SetArgs([]string{"https://path/to/test/extension"})
						assert.Nil(command.Execute())

						expectedOutput := fmt.Sprintf(inspectExtensionOutputTemplate,
							"https://path/to/test/extension",
							"REMOTE ARCHIVE:",
							simpleExtensionOutputSection)

						assert.TrimmedEq(outBuf.String(), expectedOutput)
					})
				})
			})
		})

		when("inspecting a extension on the registry", func() {
			it.Before(func() {
				simpleInfo.Location = buildpack.RegistryLocator
			})

			when("using the default registry", func() {
				it.Before(func() {
					mockClient.EXPECT().InspectExtension(client.InspectExtensionOptions{
						ExtensionName: "urn:cnb:registry:test/extension",
						Daemon:        true,
						Registry:      "default-registry",
					}).Return(simpleInfo, nil)
				})
				it("succeeds", func() {
					command.SetArgs([]string{"urn:cnb:registry:test/extension"})
					assert.Nil(command.Execute())

					expectedOutput := fmt.Sprintf(inspectExtensionOutputTemplate,
						"urn:cnb:registry:test/extension",
						"REGISTRY IMAGE:",
						simpleExtensionOutputSection)

					assert.TrimmedEq(outBuf.String(), expectedOutput)
				})
			})

			when("using a user provided registry", func() {
				it.Before(func() {
					mockClient.EXPECT().InspectExtension(client.InspectExtensionOptions{
						ExtensionName: "urn:cnb:registry:test/extension",
						Daemon:        true,
						Registry:      "some-registry",
					}).Return(simpleInfo, nil)
				})

				it("succeeds", func() {
					command.SetArgs([]string{"urn:cnb:registry:test/extension", "-r", "some-registry"})
					assert.Nil(command.Execute())

					expectedOutput := fmt.Sprintf(inspectExtensionOutputTemplate,
						"urn:cnb:registry:test/extension",
						"REGISTRY IMAGE:",
						simpleExtensionOutputSection)

					assert.TrimmedEq(outBuf.String(), expectedOutput)
				})
			})
		})

		when("a depth flag is passed", func() {
			it.Before(func() {
				complexInfo.Location = buildpack.URILocator

				mockClient.EXPECT().InspectExtension(client.InspectExtensionOptions{
					ExtensionName: "/other/path/to/test/extension",
					Daemon:        true,
					Registry:      "default-registry",
				}).Return(complexInfo, nil)
			})

			it("displays detection order to specified depth", func() {
				command.SetArgs([]string{"/other/path/to/test/extension", "-d", "2"})
				assert.Nil(command.Execute())

				assert.AssertTrimmedContains(outBuf.String(), depthExtensionOutputSection)
			})
		})
	})

	when("verbose flag is passed", func() {
		it.Before(func() {
			simpleInfo.Location = buildpack.URILocator
			mockClient.EXPECT().InspectExtension(client.InspectExtensionOptions{
				ExtensionName: "/another/path/to/test/extension",
				Daemon:        true,
				Registry:      "default-registry",
			}).Return(simpleInfo, nil)
		})

		it("displays all mixins", func() {
			command.SetArgs([]string{"/another/path/to/test/extension", "-v"})
			assert.Nil(command.Execute())

			assert.AssertTrimmedContains(outBuf.String(), simpleExtensionMixinOutputSection)
		})
	})

	when("failure cases", func() {
		when("unable to inspect extension image", func() {
			it.Before(func() {
				mockClient.EXPECT().InspectExtension(client.InspectExtensionOptions{
					ExtensionName: "failure-case/extension",
					Daemon:        true,
					Registry:      "default-registry",
				}).Return(&client.ExtensionInfo{}, errors.Wrap(image.ErrNotFound, "unable to inspect local failure-case/extension"))

				mockClient.EXPECT().InspectExtension(client.InspectExtensionOptions{
					ExtensionName: "failure-case/extension",
					Daemon:        false,
					Registry:      "default-registry",
				}).Return(&client.ExtensionInfo{}, errors.Wrap(image.ErrNotFound, "unable to inspect remote failure-case/extension"))
			})

			it("errors", func() {
				command.SetArgs([]string{"failure-case/extension"})
				err := command.Execute()
				assert.Error(err)
			})
		})
		when("unable to inspect extension archive", func() {
			it.Before(func() {
				mockClient.EXPECT().InspectExtension(client.InspectExtensionOptions{
					ExtensionName: "http://path/to/failure-case/extension",
					Daemon:        true,
					Registry:      "default-registry",
				}).Return(&client.ExtensionInfo{}, errors.New("error inspecting local archive"))

				it("errors", func() {
					command.SetArgs([]string{"http://path/to/failure-case/extension"})
					err := command.Execute()

					assert.Error(err)
					assert.Contains(err.Error(), "error inspecting local archive")
				})
			})
		})

		when("unable to inspect extension on registry", func() {
			it.Before(func() {
				mockClient.EXPECT().InspectExtension(client.InspectExtensionOptions{
					ExtensionName: "urn:cnb:registry:registry-failure/extension",
					Daemon:        true,
					Registry:      "some-registry",
				}).Return(&client.ExtensionInfo{}, errors.New("error inspecting registry image"))

				mockClient.EXPECT().InspectExtension(client.InspectExtensionOptions{
					ExtensionName: "urn:cnb:registry:registry-failure/extension",
					Daemon:        false,
					Registry:      "some-registry",
				}).Return(&client.ExtensionInfo{}, errors.New("error inspecting registry image"))
			})

			it("errors", func() {
				command.SetArgs([]string{"urn:cnb:registry:registry-failure/extension", "-r", "some-registry"})

				err := command.Execute()
				assert.Error(err)
				assert.Contains(err.Error(), "error inspecting registry image")
			})
		})
	})
}
