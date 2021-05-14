package writer_test

import (
	"bytes"
	"errors"
	"fmt"
	"testing"

	"github.com/buildpacks/lifecycle/buildpack"

	"github.com/buildpacks/pack/internal/dist"

	"github.com/buildpacks/pack/internal/inspectimage"

	"github.com/buildpacks/pack/internal/inspectimage/writer"

	"github.com/heroku/color"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	"github.com/buildpacks/pack"
	"github.com/buildpacks/pack/internal/config"
	ilogging "github.com/buildpacks/pack/internal/logging"
	h "github.com/buildpacks/pack/testhelpers"
)

func TestStructuredBOMFormat(t *testing.T) {
	color.Disable(true)
	defer color.Disable(false)
	spec.Run(t, "StructuredBOMFormat Writer", testStructuredBOMFormat, spec.Parallel(), spec.Report(report.Terminal{}))
}

func testStructuredBOMFormat(t *testing.T, when spec.G, it spec.S) {
	var (
		assert = h.NewAssertionManager(t)
		outBuf *bytes.Buffer

		remoteInfo  *pack.ImageInfo
		localInfo   *pack.ImageInfo
		generalInfo inspectimage.GeneralInfo
		logger      *ilogging.LogWithWriters
	)

	when("Print", func() {
		it.Before(func() {
			outBuf = bytes.NewBuffer(nil)
			logger = ilogging.NewLogWithWriters(outBuf, outBuf)
			remoteInfo = &pack.ImageInfo{
				BOM: []buildpack.BOMEntry{
					{
						Require: buildpack.Require{
							Name:    "remote-require",
							Version: "1.2.3",
							Metadata: map[string]interface{}{
								"cool-remote": "beans",
							},
						},
						Buildpack: buildpack.GroupBuildpack{
							ID:      "remote-buildpack",
							Version: "remote-buildpack-version",
						},
					},
				},
			}
			localInfo = &pack.ImageInfo{
				BOM: []buildpack.BOMEntry{
					{
						Require: buildpack.Require{
							Name:    "local-require",
							Version: "4.5.6",
							Metadata: map[string]interface{}{
								"cool-local": "beans",
							},
						},
						Buildpack: buildpack.GroupBuildpack{
							ID:      "local-buildpack",
							Version: "local-buildpack-version",
						},
					},
				},
			}
			generalInfo = inspectimage.GeneralInfo{
				Name: "some-image-name",
				RunImageMirrors: []config.RunImage{
					{
						Image:   "some-run-image",
						Mirrors: []string{"first-mirror", "second-mirror"},
					},
				},
			}
		})

		when("structured output", func() {
			var (
				localBomDisplay  []inspectimage.BOMEntryDisplay
				remoteBomDisplay []inspectimage.BOMEntryDisplay
			)
			it.Before(func() {
				localBomDisplay = []inspectimage.BOMEntryDisplay{{
					Name:    "local-require",
					Version: "4.5.6",
					Metadata: map[string]interface{}{
						"cool-local": "beans",
					},
					Buildpack: dist.BuildpackRef{
						BuildpackInfo: dist.BuildpackInfo{
							ID:      "local-buildpack",
							Version: "local-buildpack-version",
						},
					},
				}}
				remoteBomDisplay = []inspectimage.BOMEntryDisplay{{
					Name:    "remote-require",
					Version: "1.2.3",
					Metadata: map[string]interface{}{
						"cool-remote": "beans",
					},
					Buildpack: dist.BuildpackRef{
						BuildpackInfo: dist.BuildpackInfo{
							ID:      "remote-buildpack",
							Version: "remote-buildpack-version",
						},
					},
				}}
			})
			it("passes correct info to structuredBOMWriter", func() {
				var marshalInput interface{}

				structuredBOMWriter := writer.StructuredBOMFormat{
					MarshalFunc: func(i interface{}) ([]byte, error) {
						marshalInput = i
						return []byte("marshalled"), nil
					},
				}

				err := structuredBOMWriter.Print(logger, generalInfo, localInfo, remoteInfo, nil, nil)
				assert.Nil(err)

				assert.Equal(marshalInput, inspectimage.BOMDisplay{
					Remote: remoteBomDisplay,
					Local:  localBomDisplay,
				})
			})
			when("a localErr is passed to Print", func() {
				it("still marshals remote information", func() {
					var marshalInput interface{}

					localErr := errors.New("a local error occurred")
					structuredBOMWriter := writer.StructuredBOMFormat{
						MarshalFunc: func(i interface{}) ([]byte, error) {
							marshalInput = i
							return []byte("marshalled"), nil
						},
					}

					err := structuredBOMWriter.Print(logger, generalInfo, nil, remoteInfo, localErr, nil)
					assert.Nil(err)

					assert.Equal(marshalInput, inspectimage.BOMDisplay{
						Remote:   remoteBomDisplay,
						Local:    nil,
						LocalErr: localErr.Error(),
					})
				})
			})

			when("a remoteErr is passed to Print", func() {
				it("still marshals local information", func() {
					var marshalInput interface{}

					remoteErr := errors.New("a remote error occurred")
					structuredBOMWriter := writer.StructuredBOMFormat{
						MarshalFunc: func(i interface{}) ([]byte, error) {
							marshalInput = i
							return []byte("marshalled"), nil
						},
					}

					err := structuredBOMWriter.Print(logger, generalInfo, localInfo, nil, nil, remoteErr)
					assert.Nil(err)

					assert.Equal(marshalInput, inspectimage.BOMDisplay{
						Remote:    nil,
						Local:     localBomDisplay,
						RemoteErr: remoteErr.Error(),
					})
				})
			})
		})

		// Just test error cases, all error-free cases will be tested in JSON, TOML, and YAML subclasses.
		when("failure cases", func() {
			when("both info objects are nil", func() {
				it("displays a 'missing image' error message'", func() {
					structuredBOMWriter := writer.StructuredBOMFormat{
						MarshalFunc: testMarshalFunc,
					}

					err := structuredBOMWriter.Print(logger, generalInfo, nil, nil, nil, nil)
					assert.ErrorWithMessage(err, fmt.Sprintf("unable to find image '%s' locally or remotely", "missing-image"))
				})
			})
			when("fetching local and remote info errors", func() {
				it.Focus("returns an error", func() {
					structuredBOMWriter := writer.StructuredBOMFormat{
						MarshalFunc: func(i interface{}) ([]byte, error) {
							return []byte("cool"), nil
						},
					}
					remoteErr := errors.New("a remote error occurred")
					localErr := errors.New("a local error occurred")

					err := structuredBOMWriter.Print(logger, generalInfo, localInfo, remoteInfo, localErr, remoteErr)
					assert.ErrorContains(err, remoteErr.Error())
					assert.ErrorContains(err, localErr.Error())
				})
			})
		})
	})
}
