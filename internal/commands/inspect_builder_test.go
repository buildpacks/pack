package commands_test

import (
	"bytes"
	"errors"
	"testing"

	"github.com/Masterminds/semver"
	"github.com/golang/mock/gomock"
	"github.com/heroku/color"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"
	"github.com/spf13/cobra"

	"github.com/buildpacks/pack"
	"github.com/buildpacks/pack/internal/api"
	"github.com/buildpacks/pack/internal/builder"
	"github.com/buildpacks/pack/internal/commands"
	"github.com/buildpacks/pack/internal/commands/testmocks"
	"github.com/buildpacks/pack/internal/config"
	"github.com/buildpacks/pack/internal/dist"
	ilogging "github.com/buildpacks/pack/internal/logging"
	"github.com/buildpacks/pack/logging"
	h "github.com/buildpacks/pack/testhelpers"
)

func TestInspectBuilderCommand(t *testing.T) {
	color.Disable(true)
	defer color.Disable(false)
	spec.Run(t, "Commands", testInspectBuilderCommand, spec.Parallel(), spec.Report(report.Terminal{}))
}

func testInspectBuilderCommand(t *testing.T, when spec.G, it spec.S) {
	var (
		command        *cobra.Command
		logger         logging.Logger
		outBuf         bytes.Buffer
		mockController *gomock.Controller
		mockClient     *testmocks.MockPackClient
		cfg            config.Config
		buildpack1Info = dist.BuildpackInfo{ID: "test.bp.one", Version: "1.0.0"}
		buildpack2Info = dist.BuildpackInfo{ID: "test.bp.two", Version: "2.0.0", Homepage: "http://geocities.com/cool-bp"}
		buildpacks     = []dist.BuildpackInfo{
			buildpack1Info,
			buildpack2Info,
		}
		remoteInfo = &pack.BuilderInfo{
			Description:     "Some remote description",
			Stack:           "test.stack.id",
			Mixins:          []string{"mixin1", "mixin2", "build:mixin3", "build:mixin4"},
			RunImage:        "some/run-image",
			RunImageMirrors: []string{"first/default", "second/default"},
			Buildpacks:      buildpacks,
			Order: dist.Order{
				{Group: []dist.BuildpackRef{
					{BuildpackInfo: buildpack1Info, Optional: true},
					{BuildpackInfo: dist.BuildpackInfo{ID: buildpack2Info.ID}},
				}}},
			Lifecycle: builder.LifecycleDescriptor{
				Info: builder.LifecycleInfo{
					Version: &builder.Version{
						Version: *semver.MustParse("6.7.8"),
					},
				},
				API: builder.LifecycleAPI{
					BuildpackVersion: api.MustParse("5.6"),
					PlatformVersion:  api.MustParse("7.8"),
				},
			},
			CreatedBy: builder.CreatorMetadata{
				Name:    "Pack CLI",
				Version: "1.2.3",
			},
		}
		localInfo = &pack.BuilderInfo{
			Description:     "Some local description",
			Stack:           "test.stack.id",
			Mixins:          []string{"mixin1", "mixin2", "build:mixin3", "build:mixin4"},
			RunImage:        "some/run-image",
			RunImageMirrors: []string{"first/local-default", "second/local-default"},
			Buildpacks:      buildpacks,
			Order: dist.Order{
				{Group: []dist.BuildpackRef{{BuildpackInfo: buildpack1Info}}},
				{Group: []dist.BuildpackRef{{BuildpackInfo: dist.BuildpackInfo{ID: buildpack2Info.ID}, Optional: true}}},
			},
			Lifecycle: builder.LifecycleDescriptor{
				Info: builder.LifecycleInfo{
					Version: &builder.Version{
						Version: *semver.MustParse("4.5.6"),
					},
				},
				API: builder.LifecycleAPI{
					BuildpackVersion: api.MustParse("1.2"),
					PlatformVersion:  api.MustParse("3.4"),
				},
			},
			CreatedBy: builder.CreatorMetadata{
				Name:    "Pack CLI",
				Version: "4.5.6",
			},
		}
		remoteOutput = `
REMOTE:

Description: Some remote description

Created By:
  Name: Pack CLI
  Version: 1.2.3

Stack:
  ID: test.stack.id

Lifecycle:
  Version: 6.7.8
  Buildpack API: 5.6
  Platform API: 7.8

Run Images:
  first/local     (user-configured)
  second/local    (user-configured)
  some/run-image
  first/default
  second/default

Buildpacks:
  ID                 VERSION        HOMEPAGE
  test.bp.one        1.0.0          
  test.bp.two        2.0.0          http://geocities.com/cool-bp

Detection Order:
  Group #1:
    test.bp.one@1.0.0    (optional)
    test.bp.two          
`
		localOutput = `
LOCAL:

Description: Some local description

Created By:
  Name: Pack CLI
  Version: 4.5.6

Stack:
  ID: test.stack.id

Lifecycle:
  Version: 4.5.6
  Buildpack API: 1.2
  Platform API: 3.4

Run Images:
  first/local     (user-configured)
  second/local    (user-configured)
  some/run-image
  first/local-default
  second/local-default

Buildpacks:
  ID                 VERSION        HOMEPAGE
  test.bp.one        1.0.0          
  test.bp.two        2.0.0          http://geocities.com/cool-bp

Detection Order:
  Group #1:
    test.bp.one@1.0.0    
  Group #2:
    test.bp.two    (optional)
`
	)
	it.Before(func() {
		cfg = config.Config{
			DefaultBuilder: "default/builder",
			RunImages: []config.RunImage{
				{Image: "some/run-image", Mirrors: []string{"first/local", "second/local"}},
			},
		}
		mockController = gomock.NewController(t)
		mockClient = testmocks.NewMockPackClient(mockController)
		logger = ilogging.NewLogWithWriters(&outBuf, &outBuf)

		command = commands.InspectBuilder(logger, cfg, mockClient)
	})

	it.After(func() {
		mockController.Finish()
	})

	when("#Get", func() {
		when("remote builder image cannot be found", func() {
			it("warns 'remote image not present'", func() {
				mockClient.EXPECT().InspectBuilder("some/image", false).Return(nil, nil)
				mockClient.EXPECT().InspectBuilder("some/image", true).Return(localInfo, nil)
				command.SetArgs([]string{"some/image"})
				h.AssertNil(t, command.Execute())
				h.AssertContains(t, outBuf.String(), `Inspecting builder: 'some/image'`)
				h.AssertContains(t, outBuf.String(), "REMOTE:\n(not present)\n\n")
				h.AssertContains(t, outBuf.String(), localOutput)
			})
		})

		when("local builder image cannot be found", func() {
			it("warns 'local image not present'", func() {
				mockClient.EXPECT().InspectBuilder("some/image", false).Return(remoteInfo, nil)
				mockClient.EXPECT().InspectBuilder("some/image", true).Return(nil, nil)

				command.SetArgs([]string{"some/image"})
				h.AssertNil(t, command.Execute())
				h.AssertContains(t, outBuf.String(), `Inspecting builder: 'some/image'`)
				h.AssertContains(t, outBuf.String(), "LOCAL:\n(not present)\n")
				h.AssertContains(t, outBuf.String(), remoteOutput)
			})
		})

		when("image cannot be found", func() {
			it("logs 'errors when no image is found'", func() {
				mockClient.EXPECT().InspectBuilder("some/image", false).Return(nil, nil)
				mockClient.EXPECT().InspectBuilder("some/image", true).Return(nil, nil)

				command.SetArgs([]string{"some/image"})
				h.AssertError(t, command.Execute(), `Unable to find builder 'some/image' locally or remotely.`)
			})
		})

		when("inspector returns an error", func() {
			it("logs the error message", func() {
				mockClient.EXPECT().InspectBuilder("some/image", false).Return(nil, errors.New("some remote error"))
				mockClient.EXPECT().InspectBuilder("some/image", true).Return(nil, errors.New("some local error"))

				command.SetArgs([]string{"some/image"})
				h.AssertNil(t, command.Execute())

				h.AssertContains(t, outBuf.String(), `ERROR: inspecting remote image 'some/image': some remote error`)
				h.AssertContains(t, outBuf.String(), `ERROR: inspecting local image 'some/image': some local error`)
			})
		})

		when("the image has empty fields in info", func() {
			it.Before(func() {
				mockClient.EXPECT().InspectBuilder("some/image", false).Return(&pack.BuilderInfo{
					Stack: "test.stack.id",
				}, nil)

				mockClient.EXPECT().InspectBuilder("some/image", true).Return(&pack.BuilderInfo{
					Stack: "test.stack.id",
				}, nil)

				command.SetArgs([]string{"some/image"})
			})

			it("missing creator info is skipped", func() {
				h.AssertNil(t, command.Execute())
				h.AssertNotContains(t, outBuf.String(), "Created By:")
			})

			it("missing description is skipped", func() {
				h.AssertNil(t, command.Execute())
				h.AssertNotContains(t, outBuf.String(), "Description:")
			})

			it("missing stack mixins are skipped", func() {
				h.AssertNil(t, command.Execute())
				h.AssertNotContains(t, outBuf.String(), "Mixins")
			})

			it("missing buildpacks logs a warning", func() {
				h.AssertNil(t, command.Execute())
				h.AssertContains(t, outBuf.String(), "Buildpacks:\n  (none)")
				h.AssertContains(t, outBuf.String(), "Warning: 'some/image' has no buildpacks")
				h.AssertContains(t, outBuf.String(), "Users must supply buildpacks from the host machine")
			})

			it("missing groups logs a warning", func() {
				h.AssertNil(t, command.Execute())
				h.AssertContains(t, outBuf.String(), "Detection Order:\n  (none)")
				h.AssertContains(t, outBuf.String(), "Warning: 'some/image' does not specify detection order")
				h.AssertContains(t, outBuf.String(), "Users must build with explicitly specified buildpacks")
			})

			it("missing run image logs a warning", func() {
				h.AssertNil(t, command.Execute())
				h.AssertContains(t, outBuf.String(), "Run Images:\n  (none)")
				h.AssertContains(t, outBuf.String(), "Warning: 'some/image' does not specify a run image")
				h.AssertContains(t, outBuf.String(), "Users must build with an explicitly specified run image")
			})

			it("missing lifecycle version logs a warning", func() {
				h.AssertNil(t, command.Execute())
				h.AssertContains(t, outBuf.String(), "Warning: 'some/image' does not specify lifecycle version")
				h.AssertContains(t, outBuf.String(), "Warning: 'some/image' does not specify lifecycle buildpack api version")
				h.AssertContains(t, outBuf.String(), "Warning: 'some/image' does not specify lifecycle platform api version")
			})
		})

		when("is successful", func() {
			when("using the default builder", func() {
				it.Before(func() {
					cfg.DefaultBuilder = "some/image"
					mockClient.EXPECT().InspectBuilder("default/builder", false).Return(remoteInfo, nil)
					mockClient.EXPECT().InspectBuilder("default/builder", true).Return(localInfo, nil)
					command.SetArgs([]string{})
				})

				it("inspects the default builder", func() {
					h.AssertNil(t, command.Execute())
					h.AssertContains(t, outBuf.String(), "Inspecting default builder: 'default/builder'")
					h.AssertContains(t, outBuf.String(), remoteOutput)
					h.AssertContains(t, outBuf.String(), localOutput)
				})
			})

			when("a builder arg is passed", func() {
				it.Before(func() {
					command.SetArgs([]string{"some/image"})
					mockClient.EXPECT().InspectBuilder("some/image", false).Return(remoteInfo, nil)
					mockClient.EXPECT().InspectBuilder("some/image", true).Return(localInfo, nil)
				})

				it("displays builder information for local and remote", func() {
					h.AssertNil(t, command.Execute())
					h.AssertContains(t, outBuf.String(), "Inspecting builder: 'some/image'")
					h.AssertContains(t, outBuf.String(), remoteOutput)
					h.AssertContains(t, outBuf.String(), localOutput)
				})
			})

			when("the logger is verbose", func() {
				it.Before(func() {
					logger = ilogging.NewLogWithWriters(&outBuf, &outBuf, ilogging.WithVerbose())
					command = commands.InspectBuilder(logger, cfg, mockClient)

					cfg.DefaultBuilder = "some/image"
					mockClient.EXPECT().InspectBuilder("default/builder", false).Return(remoteInfo, nil)
					mockClient.EXPECT().InspectBuilder("default/builder", true).Return(localInfo, nil)
					command.SetArgs([]string{})
				})

				it("displays stack mixins", func() {
					stackLabels := `
Stack:
  ID: test.stack.id
  Mixins:
    mixin1
    mixin2
    build:mixin3
    build:mixin4
`

					h.AssertNil(t, command.Execute())
					h.AssertContains(t, outBuf.String(), stackLabels)
				})
			})
		})

		when("default builder is not set", func() {
			when("no builder arg is passed", func() {
				it.Before(func() {
					command = commands.InspectBuilder(logger, config.Config{}, mockClient)
					command.SetArgs([]string{})

					// expect client to fetch suggested builder descriptions
					mockClient.EXPECT().InspectBuilder(gomock.Any(), false).Return(&pack.BuilderInfo{}, nil).AnyTimes()
				})

				it("informs the user", func() {
					h.AssertNotNil(t, command.Execute())
					h.AssertContains(t, outBuf.String(), `Please select a default builder with:

	pack set-default-builder <builder-image>`)
					h.AssertMatch(t, outBuf.String(), `Paketo Buildpacks:\s+'gcr.io/paketo-buildpacks/builder:base'`)
					h.AssertMatch(t, outBuf.String(), `Paketo Buildpacks:\s+'gcr.io/paketo-buildpacks/builder:full-cf'`)
					h.AssertMatch(t, outBuf.String(), `Heroku:\s+'heroku/buildpacks:18'`)
				})
			})
		})
	})
}
