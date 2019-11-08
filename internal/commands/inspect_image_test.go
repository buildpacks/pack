package commands_test

import (
	"bytes"
	"errors"
	"regexp"
	"testing"

	"github.com/buildpack/lifecycle/metadata"
	"github.com/golang/mock/gomock"
	"github.com/heroku/color"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"
	"github.com/spf13/cobra"

	"github.com/buildpack/pack"
	"github.com/buildpack/pack/internal/commands"
	"github.com/buildpack/pack/internal/commands/testmocks"
	"github.com/buildpack/pack/internal/config"
	ilogging "github.com/buildpack/pack/internal/logging"
	"github.com/buildpack/pack/logging"
	h "github.com/buildpack/pack/testhelpers"
)

func TestInspectImageCommand(t *testing.T) {
	color.Disable(true)
	defer func() { color.Disable(false) }()
	spec.Run(t, "Commands", testInspectImageCommand, spec.Parallel(), spec.Report(report.Terminal{}))
}

func testInspectImageCommand(t *testing.T, when spec.G, it spec.S) {
	var (
		command        *cobra.Command
		logger         logging.Logger
		outBuf         bytes.Buffer
		mockController *gomock.Controller
		mockClient     *testmocks.MockPackClient
		cfg            *config.Config
	)

	it.Before(func() {
		cfg = &config.Config{}
		mockController = gomock.NewController(t)
		mockClient = testmocks.NewMockPackClient(mockController)
		logger = ilogging.NewLogWithWriters(&outBuf, &outBuf)
		command = commands.InspectImage(logger, cfg, mockClient)
		command.SetArgs([]string{"some/image"})
	})

	it.After(func() {
		mockController.Finish()
	})

	when("#InspectImage", func() {
		when("image cannot be found", func() {
			it.Before(func() {
				mockClient.EXPECT().InspectImage("some/image", false).Return(nil, nil)
				mockClient.EXPECT().InspectImage("some/image", true).Return(nil, nil)
			})

			it("logs 'Not present'", func() {
				h.AssertNil(t, command.Execute())

				h.AssertContains(t, outBuf.String(), "REMOTE:\n(not present)\n\nLOCAL:\n(not present)\n")
			})

			when("--bom", func() {
				it("adds nulls for missing images", func() {
					command.SetArgs([]string{"some/image", "--bom"})
					h.AssertNil(t, command.Execute())
					h.AssertEq(t,
						outBuf.String(),
						`{"remote":null,"local":null}`+"\n")
				})
			})
		})

		when("inspector returns an error", func() {
			it("logs the error message", func() {
				mockClient.EXPECT().InspectImage("some/image", false).Return(nil, errors.New("some remote error"))
				mockClient.EXPECT().InspectImage("some/image", true).Return(nil, errors.New("some local error"))

				command.SetArgs([]string{"some/image"})
				h.AssertNil(t, command.Execute())

				h.AssertContains(t, outBuf.String(), `ERROR: inspecting remote image 'some/image': some remote error`)
				h.AssertContains(t, outBuf.String(), `ERROR: inspecting local image 'some/image': some local error`)
			})
		})

		when("images are found", func() {
			var remoteInfo, localInfo *pack.ImageInfo

			it.Before(func() {
				remoteInfo = &pack.ImageInfo{
					StackID: "test.stack.id.remote",
					Buildpacks: []metadata.BuildpackMetadata{
						{ID: "test.bp.one.remote", Version: "1.0.0"},
						{ID: "test.bp.two.remote", Version: "2.0.0"},
					},
					Base: metadata.RunImageMetadata{
						TopLayer:  "some-remote-top-layer",
						Reference: "some-remote-run-image-reference",
					},
					Stack: metadata.StackMetadata{
						RunImage: metadata.StackRunImageMetadata{
							Image:   "some-remote-run-image",
							Mirrors: []string{"some-remote-mirror", "other-remote-mirror"},
						},
					},
					BOM: struct {
						Key1      string
						NestedKey struct {
							Key2 string
						}
					}{
						Key1: "remoteval1",
						NestedKey: struct {
							Key2 string
						}{
							Key2: "remoteval2",
						},
					},
				}
				localInfo = &pack.ImageInfo{
					StackID: "test.stack.id.local",
					Buildpacks: []metadata.BuildpackMetadata{
						{ID: "test.bp.one.local", Version: "1.0.0"},
						{ID: "test.bp.two.local", Version: "2.0.0"},
					},
					Base: metadata.RunImageMetadata{
						TopLayer:  "some-local-top-layer",
						Reference: "some-local-run-image-reference",
					},
					Stack: metadata.StackMetadata{
						RunImage: metadata.StackRunImageMetadata{
							Image:   "some-local-run-image",
							Mirrors: []string{"some-local-mirror", "other-local-mirror"},
						},
					},
					BOM: struct {
						Key1      string
						NestedKey struct {
							Key2 string
						}
					}{
						Key1: "localval1",
						NestedKey: struct {
							Key2 string
						}{
							Key2: "localval2",
						},
					},
				}
				mockClient.EXPECT().InspectImage("some/image", false).Return(remoteInfo, nil)
				mockClient.EXPECT().InspectImage("some/image", true).Return(localInfo, nil)
			})

			when("all metadata is present", func() {
				it("displays stack information for local and remote", func() {
					h.AssertNil(t, command.Execute())
					h.AssertContains(t, remoteOutput(outBuf), "Stack: test.stack.id.remote")
					h.AssertContains(t, localOutput(outBuf), "Stack: test.stack.id.local")
				})

				it("displays the base information for local and remote", func() {
					h.AssertNil(t, command.Execute())
					h.AssertContains(t, remoteOutput(outBuf), `Base Image:
  Reference: some-remote-run-image-reference
  Top Layer: some-remote-top-layer`)
					h.AssertContains(t, localOutput(outBuf), `Base Image:
  Reference: some-local-run-image-reference
  Top Layer: some-local-top-layer`)
				})

				it("displays the run image information for local and remote", func() {
					h.AssertNil(t, command.Execute())
					h.AssertContains(t, remoteOutput(outBuf), `Run Images:
  some-remote-run-image
  some-remote-mirror
  other-remote-mirror`)
					h.AssertContains(t, localOutput(outBuf), `Run Images:
  some-local-run-image
  some-local-mirror
  other-local-mirror`)
				})

				it("displays the buildpack information for local and remote", func() {
					h.AssertNil(t, command.Execute())
					h.AssertContainsMatch(t, remoteOutput(outBuf), `Buildpacks:
  ID\s+VERSION
  test.bp.one.remote\s+1.0.0
  test.bp.two.remote\s+2.0.0`)
					h.AssertContainsMatch(t, localOutput(outBuf), `Buildpacks:
  ID\s+VERSION
  test.bp.one.local\s+1.0.0
  test.bp.two.local\s+2.0.0`)
				})

				when("--bom", func() {
					it("prints the bom as JSON", func() {
						command.SetArgs([]string{"some/image", "--bom"})
						h.AssertNil(t, command.Execute())
						h.AssertEq(t,
							outBuf.String(),
							`{"remote":{"Key1":"remoteval1","NestedKey":{"Key2":"remoteval2"}},"local":{"Key1":"localval1","NestedKey":{"Key2":"localval2"}}}`+"\n")
					})
				})

				when("there are locally configured mirrors", func() {
					it.Before(func() {
						cfg.RunImages = []config.RunImage{
							{Image: "some-remote-run-image", Mirrors: []string{"first-remote-user-mirror", "second-remote-user-mirror"}},
							{Image: "some-local-run-image", Mirrors: []string{"first-local-user-mirror", "second-local-user-mirror"}},
						}
					})

					it("add the local mirrors to the run image output", func() {
						h.AssertNil(t, command.Execute())
						h.AssertContainsMatch(t, remoteOutput(outBuf), `Run Images:
  first-remote-user-mirror\s+\(user-configured\)
  second-remote-user-mirror\s+\(user-configured\)
  some-remote-run-image
  some-remote-mirror
  other-remote-mirror`)
						h.AssertContainsMatch(t, localOutput(outBuf), `Run Images:
  first-local-user-mirror\s+\(user-configured\)
  second-local-user-mirror\s+\(user-configured\)
  some-local-run-image
  some-local-mirror
  other-local-mirror`)
					})
				})
			})

			when("buildpacks are missing", func() {
				it.Before(func() {
					remoteInfo.Buildpacks = nil
					localInfo.Buildpacks = nil
				})

				it("reports that the metadata is missing", func() {
					h.AssertNil(t, command.Execute())

					h.AssertContains(t, remoteOutput(outBuf), `Buildpacks:
  (buildpacks metadata not present)`)
					h.AssertContains(t, localOutput(outBuf), `Buildpacks:
  (buildpacks metadata not present)`)
				})
			})

			when("run images are missing", func() {
				it.Before(func() {
					remoteInfo.Stack = metadata.StackMetadata{}
					localInfo.Stack = metadata.StackMetadata{}
				})

				it("reports that the metadata is missing", func() {
					h.AssertNil(t, command.Execute())

					h.AssertContains(t, remoteOutput(outBuf), `Run Images:
  (none)`)
					h.AssertContains(t, localOutput(outBuf), `Run Images:
  (none)`)
				})
			})

			when("base image metadata is missing", func() {
				it.Before(func() {
					remoteInfo.Base = metadata.RunImageMetadata{}
					localInfo.Base = metadata.RunImageMetadata{}
				})

				it("doesn't display reference field", func() {
					// runImage reference is wrong when image is generated by lifecycle pre-v0.5.0
					h.AssertNil(t, command.Execute())

					h.AssertContainsMatch(t, remoteOutput(outBuf), `(?m)Base Image:
  Top Layer:\s*$`)
					h.AssertContainsMatch(t, localOutput(outBuf), `(?m)Base Image:
  Top Layer:\s*$`)
				})
			})

			when("stack ID is missing", func() {
				it.Before(func() {
					remoteInfo.StackID = ""
					localInfo.StackID = ""
				})

				it("reports that the metadata is missing", func() {
					h.AssertNil(t, command.Execute())

					h.AssertContainsMatch(t, remoteOutput(outBuf), `(?m)Stack:\s*$`)
					h.AssertContainsMatch(t, localOutput(outBuf), `(?m)Stack:\s*$`)
				})
			})
		})
	})
}

func remoteOutput(outBuf bytes.Buffer) string {
	return regexp.MustCompile(`(?s)REMOTE:\n(.*)LOCAL:`).FindAllStringSubmatch(outBuf.String(), -1)[0][1]
}

func localOutput(outBuf bytes.Buffer) string {
	return regexp.MustCompile(`(?s)LOCAL:(.*)`).FindAllStringSubmatch(outBuf.String(), -1)[0][1]
}
