package main

import (
	"github.com/buildpack/pack/docker"
	"log"
	"os"

	"github.com/buildpack/pack/fs"

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
			return buildFlags.Run()
		},
	}
	buildCommand.Flags().StringVarP(&buildFlags.AppDir, "path", "p", wd, "path to app dir")
	buildCommand.Flags().StringVar(&buildFlags.Builder, "builder", "packs/samples", "builder")
	buildCommand.Flags().StringVar(&buildFlags.RunImage, "run-image", "packs/run", "run image")
	buildCommand.Flags().BoolVar(&buildFlags.Publish, "publish", false, "publish to registry")
	buildCommand.Flags().BoolVar(&buildFlags.NoPull, "no-pull", false, "don't pull images before use")
	return buildCommand
}

func createBuilderCommand() *cobra.Command {
	flags := pack.CreateBuilderFlags{}
	createBuilderCommand := &cobra.Command{
		Use:  "create-builder <image-name> -b <path-to-builder-toml>",
		Args: cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			flags.RepoName = args[0]

			docker, err := docker.New()
			if err != nil {
				return err
			}
			builderFactory := pack.BuilderFactory{
				FS: &fs.FS{},
				Log: log.New(os.Stdout, "", log.LstdFlags),
				Docker: docker,
			}
			builderConfig, err := builderFactory.BuilderConfigFromFlags(flags)
			if err != nil {
				return err
			}
			return builderFactory.Create(builderConfig)
		},
	}
	createBuilderCommand.Flags().BoolVar(&flags.NoPull, "no-pull", false, "don't pull stack image before use")
	createBuilderCommand.Flags().StringVarP(&flags.BuilderTomlPath, "builder-config", "b", "", "path to builder.toml file")
	return createBuilderCommand
}
