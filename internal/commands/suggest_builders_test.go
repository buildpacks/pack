package commands_test

import (
	"bytes"
	"errors"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/heroku/color"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"
	"github.com/spf13/cobra"

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
		command        *cobra.Command
		logger         logging.Logger
		outBuf         bytes.Buffer
		mockController *gomock.Controller
		mockClient     *testmocks.MockPackClient
	)

	it.Before(func() {
		mockController = gomock.NewController(t)
		mockClient = testmocks.NewMockPackClient(mockController)
		logger = ilogging.NewLogWithWriters(&outBuf, &outBuf)
		command = commands.SuggestBuilders(logger, mockClient)
	})

	when("#SuggestBuilders", func() {
		when("description metadata exists", func() {
			it.Before(func() {
				mockClient.EXPECT().InspectBuilder("cloudfoundry/cnb:tiny", false).Return(&pack.BuilderInfo{
					Description: "Tiny description",
				}, nil)
				mockClient.EXPECT().InspectBuilder("cloudfoundry/cnb:bionic", false).Return(&pack.BuilderInfo{
					Description: "Bionic description",
				}, nil)
				mockClient.EXPECT().InspectBuilder("cloudfoundry/cnb:cflinuxfs3", false).Return(&pack.BuilderInfo{
					Description: "CFLinuxFS3 description",
				}, nil)
				mockClient.EXPECT().InspectBuilder("heroku/buildpacks:18", false).Return(&pack.BuilderInfo{
					Description: "Heroku description",
				}, nil)
			})

			it("displays descriptions from metadata", func() {
				command.SetArgs([]string{})
				h.AssertNil(t, command.Execute())
				h.AssertContains(t, outBuf.String(), "Suggested builders:")
				h.AssertContainsMatch(t, outBuf.String(), `Cloud Foundry:\s+'cloudfoundry/cnb:bionic'\s+Bionic description`)
				h.AssertContainsMatch(t, outBuf.String(), `Cloud Foundry:\s+'cloudfoundry/cnb:cflinuxfs3'\s+CFLinuxFS3 description`)
				h.AssertContainsMatch(t, outBuf.String(), `Cloud Foundry:\s+'cloudfoundry/cnb:tiny'\s+Tiny description`)
				h.AssertContainsMatch(t, outBuf.String(), `Heroku:\s+'heroku/buildpacks:18'\s+Heroku description`)
			})
		})

		when("description metadata does not exist", func() {
			it.Before(func() {
				mockClient.EXPECT().InspectBuilder(gomock.Any(), false).Return(&pack.BuilderInfo{
					Description: "",
				}, nil).AnyTimes()
			})

			it("displays default descriptions", func() {
				command.SetArgs([]string{})
				h.AssertNil(t, command.Execute())
				h.AssertContains(t, outBuf.String(), "Suggested builders:")
				assertDefaultDescriptions(t, outBuf)
			})
		})

		when("error inspecting images", func() {
			it.Before(func() {
				mockClient.EXPECT().InspectBuilder(gomock.Any(), false).Return(nil, errors.New("some error")).AnyTimes()
			})

			it("displays default descriptions", func() {
				command.SetArgs([]string{})
				h.AssertNil(t, command.Execute())
				h.AssertContains(t, outBuf.String(), "Suggested builders:")
				assertDefaultDescriptions(t, outBuf)
			})
		})
	})
}

func assertDefaultDescriptions(t *testing.T, outBuf bytes.Buffer) {
	h.AssertContainsMatch(t, outBuf.String(), `Cloud Foundry:\s+'cloudfoundry/cnb:bionic'\s+Small base image with Java & Node.js`)
	h.AssertContainsMatch(t, outBuf.String(), `Cloud Foundry:\s+'cloudfoundry/cnb:cflinuxfs3'\s+Larger base image with Java, Node.js & Python`)
	h.AssertContainsMatch(t, outBuf.String(), `Cloud Foundry:\s+'cloudfoundry/cnb:tiny'\s+Tiny base image \(bionic build image, distroless run image\) with buildpacks for Golang`)
	h.AssertContainsMatch(t, outBuf.String(), `Heroku:\s+'heroku/buildpacks:18'\s+heroku-18 base image with buildpacks for Ruby, Java, Node.js, Python, Golang, & PHP`)
}
