package commands

import (
	"sort"

	"github.com/pkg/errors"
	"github.com/spf13/cobra"

	"github.com/buildpacks/pack/internal/config"
	"github.com/buildpacks/pack/internal/style"
	"github.com/buildpacks/pack/logging"
)

func trustedBuilder(logger logging.Logger, cfg config.Config, cfgPath string) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "trusted-builders",
		Short:   "Interact with trusted builders",
		Aliases: []string{"trusted-builder"},
		RunE: logError(logger, func(cmd *cobra.Command, args []string) error {
			listTrustedBuilders(logger, cfg)
			return nil
		}),
	}

	addCmd := generateAdd("trusted-builders", logger, cfg, cfgPath, addTrustedBuilder)
	addCmd.Long = "Trust builder.\n\nWhen building with this builder, all lifecycle phases will be run in a single container using the builder image."
	addCmd.Example = "pack config trusted-builders add cnbs/sample-stack-run:bionic"
	cmd.AddCommand(addCmd)

	rmCmd := generateRemove("trusted-builders", logger, cfg, cfgPath, removeTrustedBuilder)
	rmCmd.Long = "Stop trusting builder.\n\nWhen building with this builder, all lifecycle phases will be no longer be run in a single container using the builder image."
	rmCmd.Example = "pack config trusted-builders remove cnbs/sample-stack-run:bionic"
	cmd.AddCommand(rmCmd)

	listCmd := generateListCmd("trusted-builders", logger, cfg, listTrustedBuilders)
	listCmd.Long = "List Trusted Builders.\n\nShow the builders that are either trusted by default or have been explicitly trusted locally using `trust-builder`"
	listCmd.Example = "pack config trusted-builders list"
	cmd.AddCommand(listCmd)
	return cmd
}

func addTrustedBuilder(args []string, logger logging.Logger, cfg config.Config, cfgPath string) error {
	imageName := args[0]
	builderToTrust := config.TrustedBuilder{Name: imageName}

	if isTrustedBuilder(cfg, imageName) {
		logger.Infof("Builder %s is already trusted", style.Symbol(imageName))
		return nil
	}

	cfg.TrustedBuilders = append(cfg.TrustedBuilders, builderToTrust)
	if err := config.Write(cfg, cfgPath); err != nil {
		return errors.Wrap(err, "writing config")
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
		if isSuggestedBuilder(builder) {
			// Attempted to untrust a suggested builder
			return errors.Errorf("Builder %s is a suggested builder, and is trusted by default. Currently pack doesn't support making these builders untrusted", style.Symbol(builder))
		}

		logger.Infof("Builder %s wasn't trusted", style.Symbol(builder))
		return nil
	}

	err := config.Write(cfg, cfgPath)
	if err != nil {
		return errors.Wrap(err, "writing config file")
	}

	logger.Infof("Builder %s is no longer trusted", style.Symbol(builder))
	return nil
}

func listTrustedBuilders(logger logging.Logger, cfg config.Config) {
	logger.Info("Trusted Builders:")

	var trustedBuilders []string
	for _, builder := range suggestedBuilders {
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
