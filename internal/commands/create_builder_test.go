package commands_test

import (
	"bytes"
	"io/ioutil"
	"path/filepath"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/heroku/color"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"
	"github.com/spf13/cobra"

	"github.com/buildpack/pack/internal/commands"
	"github.com/buildpack/pack/internal/commands/testmocks"
	ilogging "github.com/buildpack/pack/internal/logging"
	"github.com/buildpack/pack/logging"
	h "github.com/buildpack/pack/testhelpers"
)

func TestCreateBuilderCommand(t *testing.T) {
	color.Disable(true)
	defer func() { color.Disable(false) }()
	spec.Run(t, "Commands", testCreateBuilderCommand, spec.Parallel(), spec.Report(report.Terminal{}))
}

func testCreateBuilderCommand(t *testing.T, when spec.G, it spec.S) {
	var (
		command           *cobra.Command
		logger            logging.Logger
		outBuf            bytes.Buffer
		mockController    *gomock.Controller
		mockClient        *testmocks.MockPackClient
		tmpDir            string
		builderConfigPath string
	)

	it.Before(func() {
		var err error
		tmpDir, err = ioutil.TempDir("", "create-builder-test")
		h.AssertNil(t, err)
		builderConfigPath = filepath.Join(tmpDir, "builder.toml")

		mockController = gomock.NewController(t)
		mockClient = testmocks.NewMockPackClient(mockController)
		logger = ilogging.NewLogWithWriters(&outBuf, &outBuf)
		command = commands.CreateBuilder(logger, mockClient)
	})

	it.After(func() {
		mockController.Finish()
	})

	when("#CreateBuilder", func() {
		when("warnings encountered in builder.toml", func() {
			it.Before(func() {
				h.AssertNil(t, ioutil.WriteFile(builderConfigPath, []byte(`
[[buildpacks]]
  id = "some.buildpack"
  latest = true
`), 0666))
			})

			it("logs the warnings", func() {
				mockClient.EXPECT().CreateBuilder(gomock.Any(), gomock.Any()).Return(nil)

				command.SetArgs([]string{
					"some/builder",
					"--builder-config", builderConfigPath,
				})
				h.AssertNil(t, command.Execute())

				h.AssertContains(t, outBuf.String(), "Warning: builder configuration: 'latest' field on a buildpack is obsolete and will be ignored")
				h.AssertContains(t, outBuf.String(), "Warning: builder configuration: empty 'order' definition")
			})
		})
	})
}
