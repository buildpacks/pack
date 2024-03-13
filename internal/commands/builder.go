package commands

import (
	"github.com/spf13/cobra"

	builderwriter "github.com/buildpacks/pack/internal/builder/writer"
	"github.com/buildpacks/pack/internal/config"
	"github.com/buildpacks/pack/pkg/image"
	"github.com/buildpacks/pack/pkg/logging"
)

func NewBuilderCommand(logger logging.Logger, cfg config.Config, client PackClient, imagePullPolicyHandler image.ImagePullPolicyHandler) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "builder",
		Aliases: []string{"builders"},
		Short:   "Interact with builders",
		RunE:    nil,
	}

	cmd.AddCommand(BuilderCreate(logger, cfg, client, imagePullPolicyHandler))
	cmd.AddCommand(BuilderInspect(logger, cfg, client, builderwriter.NewFactory()))
	cmd.AddCommand(BuilderSuggest(logger, client))
	AddHelpFlag(cmd, "builder")
	return cmd
}
