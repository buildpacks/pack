package commands_test

import (
	"bytes"
	"github.com/buildpack/pack"
	"github.com/buildpack/pack/config"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/pkg/errors"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"
	"github.com/spf13/cobra"

	"github.com/buildpack/pack/commands"
	cmdmocks "github.com/buildpack/pack/commands/mocks"
	"github.com/buildpack/pack/logging"
	"github.com/buildpack/pack/mocks"
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
		mockInspector  *cmdmocks.MockBuilderInspector
		mockController *gomock.Controller
		mockFetcher    *mocks.MockFetcher
	)

	it.Before(func() {
		mockController = gomock.NewController(t)
		mockInspector = cmdmocks.NewMockBuilderInspector(mockController)
		mockFetcher = mocks.NewMockFetcher(mockController)

		logger = logging.NewLogger(&outBuf, &outBuf, false, false)
		command = commands.InspectBuilder(logger, &config.Config{}, mockInspector, mockFetcher)
	})

	it.After(func() {
		mockController.Finish()
	})

	when("#InspectBuilder", func() {
		when("image cannot be found", func() {
			it("logs 'Not present'", func() {
				mockImage := mocks.NewMockImage(mockController)
				mockFetcher.EXPECT().FetchLocalImage("some/image").Return(mockImage, nil)
				mockFetcher.EXPECT().FetchRemoteImage("some/image").Return(mockImage, nil)
				mockImage.EXPECT().Found().Return(false, nil).AnyTimes()
				command.SetArgs([]string{
					"some/image",
				})
				h.AssertNil(t, command.Execute())

				h.AssertContains(t, outBuf.String(), "Remote\n------\n\nNot present\n\nLocal\n-----\n\nNot present\n")
			})
		})

		when("image fetcher returns an error", func() {
			it("logs the error message", func() {
				mockFetcher.EXPECT().FetchLocalImage("some/image").Return(nil, errors.New("some local error"))
				mockFetcher.EXPECT().FetchRemoteImage("some/image").Return(nil, errors.New("some remote error"))
				command.SetArgs([]string{
					"some/image",
				})
				h.AssertNil(t, command.Execute())

				h.AssertContains(t, outBuf.String(), `Remote
------

ERROR: failed to get image 'some/image': some remote error

Local
-----

ERROR: failed to get image 'some/image': some local error
`)
			})
		})

		when("the image is missing the stack label", func() {
			var (
				mockRemoteImage *mocks.MockImage
				mockLocalImage  *mocks.MockImage
			)

			it.Before(func() {
				mockRemoteImage = mocks.NewMockImage(mockController)
				mockRemoteImage.EXPECT().Found().Return(true, nil)
				mockRemoteImage.EXPECT().Label("io.buildpacks.stack.id").Return("", nil)
				mockFetcher.EXPECT().FetchRemoteImage("some/image").Return(mockRemoteImage, nil)

				mockLocalImage = mocks.NewMockImage(mockController)
				mockLocalImage.EXPECT().Label("io.buildpacks.stack.id").Return("", nil)
				mockLocalImage.EXPECT().Found().Return(true, nil)
				mockFetcher.EXPECT().FetchLocalImage("some/image").Return(mockLocalImage, nil)

				command.SetArgs([]string{"some/image"})
			})

			it("logs an error", func() {
				h.AssertNil(t, command.Execute())
				msg := "Error: 'some/image' is an invalid builder because it is missing a 'io.buildpacks.stack.id' label"
				h.AssertContains(t, outBuf.String(), msg)
			})
		})

		when("the image has empty fields in metadata", func() {
			var (
				mockRemoteImage *mocks.MockImage
				mockLocalImage  *mocks.MockImage
			)

			it.Before(func() {
				mockRemoteImage = mocks.NewMockImage(mockController)
				mockRemoteImage.EXPECT().Label("io.buildpacks.stack.id").Return("stack", nil)
				mockRemoteImage.EXPECT().Found().Return(true, nil)
				mockFetcher.EXPECT().FetchRemoteImage("some/image").Return(mockRemoteImage, nil)
				mockInspector.EXPECT().Inspect(mockRemoteImage).Return(pack.Builder{
					RunImage:             "run/image",
					LocalRunImageMirrors: []string{},
					RunImageMirrors:      []string{},
					Buildpacks:           []pack.BuilderBuildpackMetadata{},
					Groups:               []pack.BuilderGroupMetadata{},
				}, nil)

				mockLocalImage = mocks.NewMockImage(mockController)
				mockLocalImage.EXPECT().Label("io.buildpacks.stack.id").Return("stack", nil)
				mockLocalImage.EXPECT().Found().Return(true, nil)
				mockFetcher.EXPECT().FetchLocalImage("some/image").Return(mockLocalImage, nil)
				mockInspector.EXPECT().Inspect(mockRemoteImage).Return(pack.Builder{
					RunImage:             "run/image",
					LocalRunImageMirrors: []string{},
					RunImageMirrors:      []string{},
					Buildpacks:           []pack.BuilderBuildpackMetadata{},
					Groups:               []pack.BuilderGroupMetadata{},
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
				mockRemoteImage := mocks.NewMockImage(mockController)
				mockRemoteImage.EXPECT().Label("io.buildpacks.stack.id").Return("test.stack.id", nil)
				mockRemoteImage.EXPECT().Found().Return(true, nil)
				mockFetcher.EXPECT().FetchRemoteImage("some/image").Return(mockRemoteImage, nil)
				mockInspector.EXPECT().Inspect(mockRemoteImage).Return(pack.Builder{
					RunImage:             "run/image",
					LocalRunImageMirrors: []string{"first/image", "second/image"},
					RunImageMirrors:      []string{"first/default", "second/default"},
					Buildpacks:           []pack.BuilderBuildpackMetadata{{ID: "test.bp.one", Version: "1.0.0", Latest: true}, {ID: "fake.bp.two", Version: "2.0.0", Latest: false}},
					Groups:               []pack.BuilderGroupMetadata{{Buildpacks: []pack.BuilderBuildpackMetadata{{ID: "test.bp.one", Version: "1.0.0", Latest: true}, {ID: "fake.bp.two", Version: "2.0.0", Latest: false}}}},
				}, nil)

				mockLocalImage := mocks.NewMockImage(mockController)
				mockLocalImage.EXPECT().Label("io.buildpacks.stack.id").Return("test.stack.id", nil)
				mockLocalImage.EXPECT().Found().Return(true, nil)
				mockFetcher.EXPECT().FetchLocalImage("some/image").Return(mockLocalImage, nil)
				mockInspector.EXPECT().Inspect(mockRemoteImage).Return(pack.Builder{
					RunImage:             "run/image",
					LocalRunImageMirrors: []string{"first/local", "second/local"},
					RunImageMirrors:      []string{"first/local-default", "second/local-default"},
					Buildpacks:           []pack.BuilderBuildpackMetadata{{ID: "test.bp.one", Version: "1.0.0", Latest: true}},
					Groups: []pack.BuilderGroupMetadata{
						{Buildpacks: []pack.BuilderBuildpackMetadata{{ID: "test.bp.one", Version: "1.0.0", Latest: true}}},
						{Buildpacks: []pack.BuilderBuildpackMetadata{{ID: "fake.bp.two", Version: "2.0.0", Latest: false}}},
					},
				}, nil)
			})

			when("using the default builder", func() {
				it.Before(func() {
					command = commands.InspectBuilder(logger, &config.Config{DefaultBuilder: "some/image"}, mockInspector, mockFetcher)
				})

				it("should print a different inspection message", func() {
					command.SetArgs([]string{"some/image"})
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
  run/image
  first/default
  second/default

Buildpacks:
  ID                 VERSION        LATEST        
  test.bp.one        1.0.0          true          
  fake.bp.two        2.0.0          false

Detection Order:
  Group #1:
    test.bp.one@1.0.0
    fake.bp.two@2.0.0
`)
				h.AssertContains(t, outBuf.String(), `
Local
-----

Stack: test.stack.id

Run Images:
  first/local (user-configured)
  second/local (user-configured)
  run/image
  first/local-default
  second/local-default

Buildpacks:
  ID                 VERSION        LATEST        
  test.bp.one        1.0.0          true

Detection Order:
  Group #1:
    test.bp.one@1.0.0
  Group #2:
    fake.bp.two@2.0.0
`)
			})
		})
	})
}
