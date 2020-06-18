package commands_test

import (
	"bytes"
	"errors"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/heroku/color"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	"github.com/buildpacks/pack"
	"github.com/buildpacks/pack/internal/commands"
	"github.com/buildpacks/pack/internal/commands/testmocks"
	ilogging "github.com/buildpacks/pack/internal/logging"
	"github.com/buildpacks/pack/logging"
	h "github.com/buildpacks/pack/testhelpers"
)

func TestSuggestBuilders(t *testing.T) {
	color.Disable(true)
	defer color.Disable(false)
	spec.Run(t, "Commands", testSuggestBuildersCommand, spec.Parallel(), spec.Report(report.Terminal{}))
}

func testSuggestBuildersCommand(t *testing.T, when spec.G, it spec.S) {
	var (
		logger         logging.Logger
		outBuf         bytes.Buffer
		mockController *gomock.Controller
		mockClient     *testmocks.MockPackClient
	)

	it.Before(func() {
		mockController = gomock.NewController(t)
		mockClient = testmocks.NewMockPackClient(mockController)
		logger = ilogging.NewLogWithWriters(&outBuf, &outBuf)
	})

	when("#WriteSuggestedBuilder", func() {
		when("description metadata exists", func() {
			it.Before(func() {
				mockClient.EXPECT().InspectBuilder("gcr.io/some/builder:latest", false).Return(&pack.BuilderInfo{
					Description: "Remote description",
				}, nil)
			})

			it("displays descriptions from metadata", func() {
				commands.WriteSuggestedBuilder(logger, mockClient, []commands.SuggestedBuilder{{
					Vendor:             "Builder",
					Image:              "gcr.io/some/builder:latest",
					DefaultDescription: "Default description",
				}})
				h.AssertContains(t, outBuf.String(), "Suggested builders:")
				h.AssertContainsMatch(t, outBuf.String(), `Builder:\s+'gcr.io/some/builder:latest'\s+Remote description`)
			})
		})

		when("description metadata does not exist", func() {
			it.Before(func() {
				mockClient.EXPECT().InspectBuilder(gomock.Any(), false).Return(&pack.BuilderInfo{
					Description: "",
				}, nil).AnyTimes()
			})

			it("displays default descriptions", func() {
				commands.WriteSuggestedBuilder(logger, mockClient, []commands.SuggestedBuilder{{
					Vendor:             "Builder",
					Image:              "gcr.io/some/builder:latest",
					DefaultDescription: "Default description",
				}})
				h.AssertContains(t, outBuf.String(), "Suggested builders:")
				h.AssertContainsMatch(t, outBuf.String(), `Builder:\s+'gcr.io/some/builder:latest'\s+Default description`)
			})
		})

		when("error inspecting images", func() {
			it.Before(func() {
				mockClient.EXPECT().InspectBuilder(gomock.Any(), false).Return(nil, errors.New("some error")).AnyTimes()
			})

			it("displays default descriptions", func() {
				commands.WriteSuggestedBuilder(logger, mockClient, []commands.SuggestedBuilder{{
					Vendor:             "Builder",
					Image:              "gcr.io/some/builder:latest",
					DefaultDescription: "Default description",
				}})
				h.AssertContains(t, outBuf.String(), "Suggested builders:")
				h.AssertContainsMatch(t, outBuf.String(), `Builder:\s+'gcr.io/some/builder:latest'\s+Default description`)
			})
		})
	})
}
