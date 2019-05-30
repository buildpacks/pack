package commands_test

import (
	"bytes"
	"errors"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"
	"github.com/spf13/cobra"

	"github.com/buildpack/pack"
	"github.com/buildpack/pack/builder"
	"github.com/buildpack/pack/commands"
	cmdmocks "github.com/buildpack/pack/commands/mocks"
	"github.com/buildpack/pack/config"
	"github.com/buildpack/pack/internal/mocks"
	"github.com/buildpack/pack/logging"
	h "github.com/buildpack/pack/testhelpers"
)

func TestInspectBuilderCommand(t *testing.T) {
	spec.Run(t, "Commands", testInspectBuilderCommand, spec.Parallel(), spec.Report(report.Terminal{}))
}

func testInspectBuilderCommand(t *testing.T, when spec.G, it spec.S) {

	var (
		command        *cobra.Command
		logger         logging.Logger
		outBuf         bytes.Buffer
		mockController *gomock.Controller
		mockClient     *cmdmocks.MockPackClient
		cfg            *config.Config
	)

	it.Before(func() {
		cfg = &config.Config{}
		mockController = gomock.NewController(t)
		mockClient = cmdmocks.NewMockPackClient(mockController)
		logger = mocks.NewMockLogger(&outBuf)
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

				h.AssertContains(t, outBuf.String(), "Remote\n------\n\nNot present\n\nLocal\n-----\n\nNot present\n")
			})
		})

		when("inspector returns an error", func() {
			it("logs the error message", func() {
				mockClient.EXPECT().InspectBuilder("some/image", false).Return(nil, errors.New("some remote error"))
				mockClient.EXPECT().InspectBuilder("some/image", true).Return(nil, errors.New("some local error"))

				command.SetArgs([]string{"some/image"})
				h.AssertNil(t, command.Execute())

				h.AssertContains(t, outBuf.String(), `Remote
------

ERROR: some remote error

Local
-----

ERROR: some local error
`)
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

			it("missing description is skipped", func() {
				h.AssertNil(t, command.Execute())
				h.AssertNotContains(t, outBuf.String(), "Description:")
			})

			it("missing buildpacks logs a warning", func() {
				h.AssertNil(t, command.Execute())
				h.AssertContains(t, outBuf.String(), "Warning: 'some/image' has no buildpacks")
				h.AssertContains(t, outBuf.String(), "Users must supply buildpacks from the host machine")
			})

			it("missing groups logs a warning", func() {
				h.AssertNil(t, command.Execute())
				h.AssertContains(t, outBuf.String(), "Warning: 'some/image' does not specify detection order")
				h.AssertContains(t, outBuf.String(), "Users must build with explicitly specified buildpacks")
			})

			it("missing run image logs a warning", func() {
				h.AssertNil(t, command.Execute())
				h.AssertContains(t, outBuf.String(), "Warning: 'some/image' does not specify a run image")
				h.AssertContains(t, outBuf.String(), "Users must build with an explicitly specified run image")
			})

			it("missing lifecycle version prints Unknown", func() {
				h.AssertNil(t, command.Execute())
				h.AssertContains(t, outBuf.String(), "Lifecycle Version: Unknown")
			})
		})

		when("is successful", func() {
			it.Before(func() {
				buildpacks := []builder.BuildpackMetadata{
					{ID: "test.bp.one", Version: "1.0.0", Latest: true},
					{ID: "test.bp.two", Version: "2.0.0", Latest: false},
				}
				remoteInfo := &pack.BuilderInfo{
					Description:          "Some remote description",
					Stack:                "test.stack.id",
					RunImage:             "some/run-image",
					RunImageMirrors:      []string{"first/default", "second/default"},
					LocalRunImageMirrors: []string{"first/image", "second/image"},
					Buildpacks:           buildpacks,
					Groups: []builder.GroupMetadata{
						{Buildpacks: []builder.GroupBuildpack{
							{ID: "test.bp.one", Version: "1.0.0", Optional: true},
							{ID: "test.bp.two", Version: "2.0.0"},
						}}},
					LifecycleVersion: "6.7.8",
				}
				mockClient.EXPECT().InspectBuilder("some/image", false).Return(remoteInfo, nil)

				localInfo := &pack.BuilderInfo{
					Description:          "Some local description",
					Stack:                "test.stack.id",
					RunImage:             "some/run-image",
					RunImageMirrors:      []string{"first/local-default", "second/local-default"},
					LocalRunImageMirrors: []string{"first/local", "second/local"},
					Buildpacks:           buildpacks,
					Groups: []builder.GroupMetadata{
						{Buildpacks: []builder.GroupBuildpack{{ID: "test.bp.one", Version: "1.0.0"}}},
						{Buildpacks: []builder.GroupBuildpack{{ID: "test.bp.two", Version: "2.0.0", Optional: true}}},
					},
					LifecycleVersion: "4.5.6",
				}
				mockClient.EXPECT().InspectBuilder("some/image", true).Return(localInfo, nil)
			})

			when("using the default builder", func() {
				it.Before(func() {
					cfg.DefaultBuilder = "some/image"
					command.SetArgs([]string{})
				})

				it("should print a different inspection message", func() {
					h.AssertNil(t, command.Execute())
					h.AssertContains(t, outBuf.String(), "Inspecting default builder: 'some/image'")
				})
			})

			it("displays builder information for local and remote", func() {
				command.SetArgs([]string{"some/image"})
				h.AssertNil(t, command.Execute())
				h.AssertContains(t, outBuf.String(), "Inspecting builder: 'some/image'")
				h.AssertContains(t, outBuf.String(), `
Remote
------

Description: Some remote description

Stack: test.stack.id

Lifecycle Version: 6.7.8

Run Images:
  first/image (user-configured)
  second/image (user-configured)
  some/run-image
  first/default
  second/default

Buildpacks:
  ID                 VERSION        LATEST
  test.bp.one        1.0.0          true
  test.bp.two        2.0.0          false

Detection Order:
  Group #1:
    test.bp.one@1.0.0    (optional)
    test.bp.two@2.0.0
`)

				h.AssertContains(t, outBuf.String(), `
Local
-----

Description: Some local description

Stack: test.stack.id

Lifecycle Version: 4.5.6

Run Images:
  first/local (user-configured)
  second/local (user-configured)
  some/run-image
  first/local-default
  second/local-default

Buildpacks:
  ID                 VERSION        LATEST
  test.bp.one        1.0.0          true
  test.bp.two        2.0.0          false

Detection Order:
  Group #1:
    test.bp.one@1.0.0
  Group #2:
    test.bp.two@2.0.0    (optional)
`)
			})
		})

		when("default builder is not set", func() {
			it.Before(func() {
				mockClient.EXPECT().InspectBuilder(gomock.Any(), false).Return(&pack.BuilderInfo{}, nil).AnyTimes()
			})

			it("informs the user", func() {
				command.SetArgs([]string{})
				h.AssertNotNil(t, command.Execute())
				h.AssertContains(t, outBuf.String(), `Please select a default builder with:

	pack set-default-builder <builder image>`)
				h.AssertMatch(t, outBuf.String(), `Cloud Foundry:\s+'cloudfoundry/cnb:bionic'`)
				h.AssertMatch(t, outBuf.String(), `Cloud Foundry:\s+'cloudfoundry/cnb:cflinuxfs3'`)
				h.AssertMatch(t, outBuf.String(), `Heroku:\s+'heroku/buildpacks'`)
			})
		})
	})
}
