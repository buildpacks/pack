package main

import (
	"os"

	"github.com/buildpack/pack"
	"github.com/spf13/cobra"
)

func main() {
	buildCmd := buildCommand()
	createBuilderCmd := createBuilderCommand()

	rootCmd := &cobra.Command{Use: "pack"}
	rootCmd.AddCommand(buildCmd)
	rootCmd.AddCommand(createBuilderCmd)
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func buildCommand() *cobra.Command {
	wd, _ := os.Getwd()

	var buildFlags pack.BuildFlags
	buildCommand := &cobra.Command{
		Use:  "build <image-name>",
		Args: cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			buildFlags.RepoName = args[0]
			if err := buildFlags.Init(); err != nil {
				return err
			}
			defer buildFlags.Close()
			return buildFlags.Run()
		},
	}
	buildCommand.Flags().StringVarP(&buildFlags.AppDir, "path", "p", wd, "path to app dir")
	buildCommand.Flags().StringVar(&buildFlags.Builder, "builder", "dgodd/packsv3samples:dev", "builder")
	buildCommand.Flags().StringVar(&buildFlags.RunImage, "run-image", "dgodd/packsv3run:dev", "run image")
	buildCommand.Flags().BoolVar(&buildFlags.Publish, "publish", false, "publish to registry")
	buildCommand.Flags().BoolVar(&buildFlags.NoPull, "no-pull", false, "don't pull images before use")
	return buildCommand
}

func createBuilderCommand() *cobra.Command {
	builderFactory := pack.BuilderFactory{
		DefaultStack: pack.Stack{
			ID:         "",
			BuildImage: "packs/build",
			RunImage:   "packs/run",
		},
	}

	var createBuilderFlags pack.CreateBuilderFlags
	createBuilderCommand := &cobra.Command{
		Use:  "create-builder <image-name> -b <path-to-builder-toml>",
		Args: cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			createBuilderFlags.RepoName = args[0]
			return builderFactory.Create(createBuilderFlags)
		},
	}
	createBuilderCommand.Flags().StringVarP(&createBuilderFlags.BuilderTomlPath, "builder-config", "b", "", "path to builder.toml file")
	return createBuilderCommand
}
