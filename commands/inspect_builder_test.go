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

	"github.com/buildpack/pack"
	"github.com/buildpack/pack/api"
	"github.com/buildpack/pack/builder"
	"github.com/buildpack/pack/commands"
	cmdmocks "github.com/buildpack/pack/commands/mocks"
	"github.com/buildpack/pack/config"
	ilogging "github.com/buildpack/pack/internal/logging"
	"github.com/buildpack/pack/logging"
	h "github.com/buildpack/pack/testhelpers"
)

func TestInspectBuilderCommand(t *testing.T) {
	color.Disable(true)
	defer func() { color.Disable(false) }()
	spec.Run(t, "Commands", testInspectBuilderCommand, spec.Parallel(), spec.Report(report.Terminal{}))
}

func testInspectBuilderCommand(t *testing.T, when spec.G, it spec.S) {

	var (
		command        *cobra.Command
		logger         logging.Logger
		outBuf         bytes.Buffer
		mockController *gomock.Controller
		mockClient     *cmdmocks.MockPackClient
		cfg            config.Config
	)

	it.Before(func() {
		cfg = config.Config{
			DefaultBuilder: "default/builder",
			RunImages: []config.RunImage{
				{Image: "some/run-image", Mirrors: []string{"first/local", "second/local"}},
			},
		}
		mockController = gomock.NewController(t)
		mockClient = cmdmocks.NewMockPackClient(mockController)
		logger = ilogging.NewLogWithWriters(&outBuf, &outBuf)

		command = commands.InspectBuilder(logger, cfg, mockClient)
	})

	it.After(func() {
		mockController.Finish()
	})

	when("#GetBuilder", func() {
		when("image cannot be found", func() {
			it("logs 'Not present'", func() {
				mockClient.EXPECT().InspectBuilder("some/image", false).Return(nil, nil)
				mockClient.EXPECT().InspectBuilder("some/image", true).Return(nil, nil)

				command.SetArgs([]string{"some/image"})
				h.AssertNil(t, command.Execute())

				h.AssertContains(t, outBuf.String(), "REMOTE:\n(not present)\n\nLOCAL:\n(not present)\n")
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

			it("missing lifecycle version prints assumed", func() {
				h.AssertNil(t, command.Execute())
				h.AssertContains(t, outBuf.String(), "Lifecycle:\n  Version: 0.3.0")
			})
		})

		when("is successful", func() {
			var (
				buildpack1Info = builder.BuildpackInfo{ID: "test.bp.one", Version: "1.0.0"}
				buildpack2Info = builder.BuildpackInfo{ID: "test.bp.two", Version: "2.0.0"}
				buildpacks     = []builder.BuildpackMetadata{
					{BuildpackInfo: buildpack1Info, Latest: true},
					{BuildpackInfo: buildpack2Info, Latest: false},
				}
				remoteInfo = &pack.BuilderInfo{
					Description:     "Some remote description",
					Stack:           "test.stack.id",
					RunImage:        "some/run-image",
					RunImageMirrors: []string{"first/default", "second/default"},
					Buildpacks:      buildpacks,
					Groups: builder.Order{
						{Group: []builder.BuildpackRef{
							{BuildpackInfo: buildpack1Info, Optional: true},
							{BuildpackInfo: builder.BuildpackInfo{ID: buildpack2Info.ID}},
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
					RunImage:        "some/run-image",
					RunImageMirrors: []string{"first/local-default", "second/local-default"},
					Buildpacks:      buildpacks,
					Groups: builder.Order{
						{Group: []builder.BuildpackRef{{BuildpackInfo: buildpack1Info}}},
						{Group: []builder.BuildpackRef{{BuildpackInfo: builder.BuildpackInfo{ID: buildpack2Info.ID}, Optional: true}}},
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
			)

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
					h.AssertContains(t, outBuf.String(), `
REMOTE:

Description: Some remote description

Created By:
  Name: Pack CLI
  Version: 1.2.3

Stack: test.stack.id

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
  ID                 VERSION
  test.bp.one        1.0.0
  test.bp.two        2.0.0

Detection Order:
  Group #1:
    test.bp.one@1.0.0    (optional)
    test.bp.two          
`)

					h.AssertContains(t, outBuf.String(), `
LOCAL:

Description: Some local description

Created By:
  Name: Pack CLI
  Version: 4.5.6

Stack: test.stack.id

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
  ID                 VERSION
  test.bp.one        1.0.0
  test.bp.two        2.0.0

Detection Order:
  Group #1:
    test.bp.one@1.0.0    
  Group #2:
    test.bp.two    (optional)
`)
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
					h.AssertContains(t, outBuf.String(), `
REMOTE:

Description: Some remote description

Created By:
  Name: Pack CLI
  Version: 1.2.3

Stack: test.stack.id

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
  ID                 VERSION
  test.bp.one        1.0.0
  test.bp.two        2.0.0

Detection Order:
  Group #1:
    test.bp.one@1.0.0    (optional)
    test.bp.two          
`)

					h.AssertContains(t, outBuf.String(), `
LOCAL:

Description: Some local description

Created By:
  Name: Pack CLI
  Version: 4.5.6

Stack: test.stack.id

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
  ID                 VERSION
  test.bp.one        1.0.0
  test.bp.two        2.0.0

Detection Order:
  Group #1:
    test.bp.one@1.0.0    
  Group #2:
    test.bp.two    (optional)
`)
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

	pack set-default-builder <builder image>`)
					h.AssertMatch(t, outBuf.String(), `Cloud Foundry:\s+'cloudfoundry/cnb:bionic'`)
					h.AssertMatch(t, outBuf.String(), `Cloud Foundry:\s+'cloudfoundry/cnb:cflinuxfs3'`)
					h.AssertMatch(t, outBuf.String(), `Heroku:\s+'heroku/buildpacks:18'`)
				})
			})
		})
	})
}
