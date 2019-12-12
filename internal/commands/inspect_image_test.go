package commands_test

import (
	"bytes"
	"errors"
	"io/ioutil"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/buildpacks/lifecycle"
	"github.com/golang/mock/gomock"
	"github.com/heroku/color"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"
	"github.com/spf13/cobra"

	"github.com/buildpacks/pack"
	"github.com/buildpacks/pack/internal/commands"
	"github.com/buildpacks/pack/internal/commands/testmocks"
	"github.com/buildpacks/pack/internal/config"
	ilogging "github.com/buildpacks/pack/internal/logging"
	"github.com/buildpacks/pack/logging"
	h "github.com/buildpacks/pack/testhelpers"
)

func TestInspectImageCommand(t *testing.T) {
	color.Disable(true)
	defer color.Disable(false)
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
				type someData struct {
					String string
					Bool   bool
					Int    int
					Nested struct {
						String string
					}
				}

				remoteInfo = &pack.ImageInfo{
					StackID: "test.stack.id.remote",
					Buildpacks: []lifecycle.Buildpack{
						{ID: "test.bp.one.remote", Version: "1.0.0"},
						{ID: "test.bp.two.remote", Version: "2.0.0"},
					},
					Base: lifecycle.RunImageMetadata{
						TopLayer:  "some-remote-top-layer",
						Reference: "some-remote-run-image-reference",
					},
					Stack: lifecycle.StackMetadata{
						RunImage: lifecycle.StackRunImageMetadata{
							Image:   "some-remote-run-image",
							Mirrors: []string{"some-remote-mirror", "other-remote-mirror"},
						},
					},
					BOM: []lifecycle.BOMEntry{{
						Require: lifecycle.Require{
							Name:    "name-1",
							Version: "version-1",
							Metadata: map[string]interface{}{
								"RemoteData": someData{
									String: "aString",
									Bool:   true,
									Int:    123,
									Nested: struct {
										String string
									}{
										String: "anotherString",
									},
								},
							},
						},
						Buildpack: lifecycle.Buildpack{ID: "test.bp.one.remote", Version: "1.0.0"},
					}},
					Processes: pack.ProcessDetails{
						DefaultProcess: &lifecycle.Process{
							Type:    "some-remote-type",
							Command: "/some/remote command",
							Args:    []string{"some", "remote", "args"},
							Direct:  false,
						},
						OtherProcesses: []lifecycle.Process{
							{
								Type:    "other-remote-type",
								Command: "/other/remote/command",
								Args:    []string{"other", "remote", "args"},
								Direct:  true,
							},
						},
					},
				}
				localInfo = &pack.ImageInfo{
					StackID: "test.stack.id.local",
					Buildpacks: []lifecycle.Buildpack{
						{ID: "test.bp.one.local", Version: "1.0.0"},
						{ID: "test.bp.two.local", Version: "2.0.0"},
					},
					Base: lifecycle.RunImageMetadata{
						TopLayer:  "some-local-top-layer",
						Reference: "some-local-run-image-reference",
					},
					Stack: lifecycle.StackMetadata{
						RunImage: lifecycle.StackRunImageMetadata{
							Image:   "some-local-run-image",
							Mirrors: []string{"some-local-mirror", "other-local-mirror"},
						},
					},
					BOM: []lifecycle.BOMEntry{{
						Require: lifecycle.Require{
							Name:    "name-1",
							Version: "version-1",
							Metadata: map[string]interface{}{
								"LocalData": someData{
									Bool: false,
									Int:  456,
								},
							},
						},
						Buildpack: lifecycle.Buildpack{ID: "test.bp.one.remote", Version: "1.0.0"},
					}},
					Processes: pack.ProcessDetails{
						DefaultProcess: &lifecycle.Process{
							Type:    "some-local-type",
							Command: "/some/local command",
							Args:    []string{"some", "local", "args"},
							Direct:  false,
						},
						OtherProcesses: []lifecycle.Process{
							{
								Type:    "other-local-type",
								Command: "/other/local/command",
								Args:    []string{"other", "local", "args"},
								Direct:  true,
							},
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

				it("displays process info for local and remote", func() {
					h.AssertNil(t, command.Execute())
					h.AssertContainsMatch(t, remoteOutput(outBuf), `Processes:
  TYPE\s+SHELL\s+COMMAND\s+ARGS
  some-remote-type \(default\)\s+bash\s+/some/remote command\s+some remote args
  other-remote-type\s+/other/remote/command\s+other remote args`)
					h.AssertContainsMatch(t, localOutput(outBuf), `Processes:
  TYPE\s+SHELL\s+COMMAND\s+ARGS
  some-local-type \(default\)\s+bash\s+/some/local command\s+some local args
  other-local-type\s+/other/local/command\s+other local args`)
				})

				when("there are no default processes", func() {
					it.Before(func() {
						remoteInfo.Processes.DefaultProcess = nil
						localInfo.Processes.DefaultProcess = nil
					})

					it("displays all local and remote processes with no default label", func() {
						h.AssertNil(t, command.Execute())
						h.AssertContainsMatch(t, remoteOutput(outBuf), `Processes:
  TYPE\s+SHELL\s+COMMAND\s+ARGS
  other-remote-type\s+/other/remote/command\s+other remote args`)
						h.AssertContainsMatch(t, localOutput(outBuf), `Processes:
  TYPE\s+SHELL\s+COMMAND\s+ARGS
  other-local-type\s+/other/local/command\s+other local args`)
					})
				})

				when("--bom", func() {
					it("prints the bom as JSON", func() {
						command.SetArgs([]string{"some/image", "--bom"})
						h.AssertNil(t, command.Execute())
						expectedOutput, err := ioutil.ReadFile(filepath.Join("testdata", "inspect_image_output.json"))
						h.AssertNil(t, err)
						h.AssertEq(t, strings.TrimSpace(outBuf.String()), string(expectedOutput))
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
					remoteInfo.Stack = lifecycle.StackMetadata{}
					localInfo.Stack = lifecycle.StackMetadata{}
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
					remoteInfo.Base = lifecycle.RunImageMetadata{}
					localInfo.Base = lifecycle.RunImageMetadata{}
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
