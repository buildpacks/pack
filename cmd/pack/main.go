package main

import (
	"os"

	"github.com/buildpack/pack"
	"github.com/spf13/cobra"
)

func main() {
	wd, _ := os.Getwd()

	var buildFlags struct {
		appDir      string
		detectImage string
		publish     bool
	}
	buildCommand := &cobra.Command{
		Use:  "build [IMAGE NAME]",
		Args: cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			repoName := args[0]
			return pack.Build(buildFlags.appDir, buildFlags.detectImage, repoName, buildFlags.publish)
		},
	}
	buildCommand.Flags().StringVarP(&buildFlags.appDir, "path", "p", wd, "path to app dir")
	buildCommand.Flags().StringVar(&buildFlags.detectImage, "detect-image", "packs/v3:detect", "detect image")
	buildCommand.Flags().BoolVar(&buildFlags.publish, "publish", false, "publish to registry")

	var createFlags pack.Create
	createCommand := &cobra.Command{
		Use:  "create [DETECT IMAGE NAME] [BUILD IMAGE NAME]",
		Args: cobra.MinimumNArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			createFlags.DetectImage = args[0]
			createFlags.BuildImage = args[1]
			return createFlags.Run()
		},
	}
	createCommand.Flags().StringVarP(&createFlags.BPDir, "path", "p", wd, "path to dir with buildpacks and order.toml")
	createCommand.Flags().StringVar(&createFlags.BaseImage, "from-base-image", "packs/v3:latest", "from base image")
	createCommand.Flags().BoolVar(&createFlags.Publish, "publish", false, "publish to registry")

	rootCmd := &cobra.Command{Use: "pack"}
	rootCmd.AddCommand(buildCommand)
	rootCmd.AddCommand(createCommand)
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
