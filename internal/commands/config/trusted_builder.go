package config

import (
	"sort"

	"github.com/pkg/errors"

	"github.com/buildpacks/pack/internal/commands"
	"github.com/buildpacks/pack/internal/style"

	"github.com/buildpacks/pack/internal/config"
	"github.com/buildpacks/pack/logging"
	"github.com/spf13/cobra"
)

func trustedBuilder(logger logging.Logger, cfg config.Config, cfgPath string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "trusted-builder",
		Args:  cobra.ExactArgs(1),
		Short: "Interact with trusted builders",
		RunE: commands.LogError(logger, func(cmd *cobra.Command, args []string) error {
			listTrustedBuilders(logger, cfg)
			return nil
		}),
	}

	addCmd := generateAdd("trusted-builder", logger, cfg, cfgPath, addTrustedBuilder)
	cmd.AddCommand(addCmd)

	rmCmd := generateRemove("trusted-builder", logger, cfg, cfgPath, removeTrustedBuilder)
	cmd.AddCommand(rmCmd)

	cmd.AddCommand(generateListCmd("trusted-builders", logger, cfg, listTrustedBuilders))
	return cmd
}

func addTrustedBuilder(args []string, logger logging.Logger, cfg config.Config, cfgPath string) error {
	imageName := args[0]
	builderToTrust := config.TrustedBuilder{Name: imageName}

	if commands.IsTrustedBuilder(cfg, imageName) {
		logger.Infof("Builder %s is already trusted", style.Symbol(imageName))
		return nil
	}

	cfg.TrustedBuilders = append(cfg.TrustedBuilders, builderToTrust)
	if err := config.Write(cfg, cfgPath); err != nil {
		return err
	}
	logger.Infof("Builder %s is now trusted", style.Symbol(imageName))

	return nil
}

func removeTrustedBuilder(args []string, logger logging.Logger, cfg config.Config, cfgPath string) error {
	builder := args[0]

	existingTrustedBuilders := cfg.TrustedBuilders
	cfg.TrustedBuilders = []config.TrustedBuilder{}
	for _, trustedBuilder := range existingTrustedBuilders {
		if trustedBuilder.Name == builder {
			continue
		}

		cfg.TrustedBuilders = append(cfg.TrustedBuilders, trustedBuilder)
	}

	// Builder is not in the trusted builder list
	if len(existingTrustedBuilders) == len(cfg.TrustedBuilders) {
		if commands.IsSuggestedBuilder(builder) {
			// Attempted to untrust a suggested builder
			return errors.Errorf("Builder %s is a suggested builder, and is trusted by default. Currently pack doesn't support making these builders untrusted", style.Symbol(builder))
		}

		logger.Infof("Builder %s wasn't trusted", style.Symbol(builder))
		return nil
	}

	configPath, err := config.DefaultConfigPath()
	if err != nil {
		return errors.Wrap(err, "getting config path")
	}
	err = config.Write(cfg, configPath)
	if err != nil {
		return errors.Wrap(err, "writing config file")
	}

	logger.Infof("Builder %s is no longer trusted", style.Symbol(builder))
	return nil
}

func listTrustedBuilders(logger logging.Logger, cfg config.Config) {
	logger.Info("Trusted Builders:")

	var trustedBuilders []string
	for _, builder := range commands.SuggestedBuilders {
		trustedBuilders = append(trustedBuilders, builder.Image)
	}

	for _, builder := range cfg.TrustedBuilders {
		trustedBuilders = append(trustedBuilders, builder.Name)
	}

	sort.Strings(trustedBuilders)

	for _, builder := range trustedBuilders {
		logger.Infof("  %s", builder)
	}
}
