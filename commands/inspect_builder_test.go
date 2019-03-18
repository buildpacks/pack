package commands_test

import (
	"bytes"
	"errors"
	"github.com/buildpack/pack"
	"github.com/buildpack/pack/config"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"
	"github.com/spf13/cobra"

	"github.com/buildpack/pack/commands"
	cmdmocks "github.com/buildpack/pack/commands/mocks"
	"github.com/buildpack/pack/logging"
	h "github.com/buildpack/pack/testhelpers"
)

func TestCommands(t *testing.T) {
	spec.Run(t, "Commands", testCommands, spec.Parallel(), spec.Report(report.Terminal{}))
}

func testCommands(t *testing.T, when spec.G, it spec.S) {

	var (
		command        *cobra.Command
		logger         *logging.Logger
		outBuf         bytes.Buffer
		mockController *gomock.Controller
		mockInspector  *cmdmocks.MockBuilderInspector
		cfg            *config.Config
	)

	it.Before(func() {
		cfg = &config.Config{}
		mockController = gomock.NewController(t)
		mockInspector = cmdmocks.NewMockBuilderInspector(mockController)
		logger = logging.NewLogger(&outBuf, &outBuf, false, false)
		command = commands.InspectBuilder(logger, cfg, mockInspector)
	})

	it.After(func() {
		mockController.Finish()
	})

	when("#InspectBuilder", func() {
		when("image cannot be found", func() {
			it("logs 'Not present'", func() {
				mockInspector.EXPECT().InspectBuilder("some/image", false).Return(nil, nil)
				mockInspector.EXPECT().InspectBuilder("some/image", true).Return(nil, nil)

				command.SetArgs([]string{"some/image"})
				h.AssertNil(t, command.Execute())

				h.AssertContains(t, outBuf.String(), "Remote\n------\n\nNot present\n\nLocal\n-----\n\nNot present\n")
			})
		})

		when("inspector returns an error", func() {
			it("logs the error message", func() {
				mockInspector.EXPECT().InspectBuilder("some/image", false).Return(nil, errors.New("some remote error"))
				mockInspector.EXPECT().InspectBuilder("some/image", true).Return(nil, errors.New("some local error"))

				command.SetArgs([]string{"some/image"})
				h.AssertNil(t, command.Execute())

				h.AssertContains(t, outBuf.String(), `Remote
------

ERROR: failed to inspect image 'some/image': some remote error

Local
-----

ERROR: failed to inspect image 'some/image': some local error
`)
			})
		})

		when("the image has empty fields in info", func() {
			it.Before(func() {
				mockInspector.EXPECT().InspectBuilder("some/image", false).Return(&pack.BuilderInfo{
					Stack: "test.stack.id",
				}, nil)

				mockInspector.EXPECT().InspectBuilder("some/image", true).Return(&pack.BuilderInfo{
					Stack: "test.stack.id",
				}, nil)

				command.SetArgs([]string{"some/image"})
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
		})

		when("is successful", func() {
			it.Before(func() {
				buildpacks := []pack.BuildpackInfo{
					{ID: "test.bp.one", Version: "1.0.0", Latest: true},
					{ID: "test.bp.two", Version: "2.0.0", Latest: false},
				}
				remoteInfo := &pack.BuilderInfo{
					Stack:                "test.stack.id",
					RunImage:             "some/run-image",
					RunImageMirrors:      []string{"first/default", "second/default"},
					LocalRunImageMirrors: []string{"first/image", "second/image"},
					Buildpacks:           buildpacks,
					Groups:               [][]pack.BuildpackInfo{buildpacks},
				}
				mockInspector.EXPECT().InspectBuilder("some/image", false).Return(remoteInfo, nil)

				localInfo := &pack.BuilderInfo{
					Stack:                "test.stack.id",
					RunImage:             "some/run-image",
					RunImageMirrors:      []string{"first/local-default", "second/local-default"},
					LocalRunImageMirrors: []string{"first/local", "second/local"},
					Buildpacks:           buildpacks,
					Groups:               [][]pack.BuildpackInfo{{buildpacks[0]}, {buildpacks[1]}},
				}
				mockInspector.EXPECT().InspectBuilder("some/image", true).Return(localInfo, nil)
			})

			when("using the default builder", func() {
				it.Before(func() {
					cfg.DefaultBuilder = "some/image"
					command.SetArgs([]string{})
				})

				it("should print a different inspection message", func() {
					h.AssertNil(t, command.Execute())
					h.AssertContains(t, outBuf.String(), "Inspecting default builder: some/image")
				})
			})

			it("displays builder information for local and remote", func() {
				command.SetArgs([]string{"some/image"})
				h.AssertNil(t, command.Execute())
				h.AssertContains(t, outBuf.String(), "Inspecting builder: some/image")
				h.AssertContains(t, outBuf.String(), `
Remote
------

Stack: test.stack.id

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
    test.bp.one@1.0.0
    test.bp.two@2.0.0
`)

				h.AssertContains(t, outBuf.String(), `
Local
-----

Stack: test.stack.id

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
    test.bp.two@2.0.0
`)
			})
		})
	})
}
