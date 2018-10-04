package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/buildpack/pack/config"
	"github.com/buildpack/pack/docker"
	"github.com/buildpack/pack/image"

	"github.com/buildpack/pack/fs"

	"github.com/buildpack/pack"
	"github.com/spf13/cobra"
)

func main() {
	buildCmd := buildCommand()
	createBuilderCmd := createBuilderCommand()
	addStackCmd := addStackCommand()
	deleteStackCmd := deleteStackCommand()

	rootCmd := &cobra.Command{Use: "pack"}
	rootCmd.AddCommand(buildCmd)
	rootCmd.AddCommand(createBuilderCmd)
	rootCmd.AddCommand(addStackCmd)
	rootCmd.AddCommand(deleteStackCmd)
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
			bf, err := pack.DefaultBuildFactory()
			if err != nil {
				return err
			}
			b, err := bf.BuildConfigFromFlags(&buildFlags)
			if err != nil {
				return err
			}
			return b.Run()
		},
	}
	buildCommand.Flags().StringVarP(&buildFlags.AppDir, "path", "p", wd, "path to app dir")
	buildCommand.Flags().StringVar(&buildFlags.Builder, "builder", "packs/samples", "builder")
	buildCommand.Flags().StringVar(&buildFlags.RunImage, "run-image", "", "run image")
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
			cfg, err := config.New(filepath.Join(os.Getenv("HOME"), ".pack"))
			if err != nil {
				return err
			}
			builderFactory := pack.BuilderFactory{
				FS:     &fs.FS{},
				Log:    log.New(os.Stdout, "", log.LstdFlags),
				Docker: docker,
				Config: cfg,
				Images: &image.Client{},
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
	createBuilderCommand.Flags().StringVarP(&flags.StackID, "stack", "s", "", "stack ID")
	return createBuilderCommand
}

func addStackCommand() *cobra.Command {
	flags := struct {
		BuildImage string
		RunImage   string
	}{}
	addStackCommand := &cobra.Command{
		Use:  "add-stack <stack-name> --run-image=<name> --build-image=<name>",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.New(filepath.Join(os.Getenv("HOME"), ".pack"))
			if err != nil {
				return err
			}
			if err := cfg.Add(config.Stack{
				ID:          args[0],
				BuildImages: []string{flags.BuildImage},
				RunImages:   []string{flags.RunImage},
			}); err != nil {
				return err
			}
			fmt.Printf("%s successfully added\n", args[0])
			return nil
		},
	}
	addStackCommand.Flags().StringVar(&flags.BuildImage, "build-image", "", "build image to be used for bulder images built with the stack")
	addStackCommand.Flags().StringVar(&flags.RunImage, "run-image", "", "run image to be used for runnable images built with the stack")
	return addStackCommand
}

func deleteStackCommand() *cobra.Command {
	addStackCommand := &cobra.Command{
		Use:  "delete-stack <stack-name>",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.New(filepath.Join(os.Getenv("HOME"), ".pack"))
			if err != nil {
				return err
			}
			if err := cfg.Delete(args[0]); err != nil {
				return err
			}
			fmt.Printf("%s has been successfully deleted\n", args[0])
			return nil
		},
	}
	return addStackCommand
}
