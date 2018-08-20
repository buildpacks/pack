package main

import (
	"os"

	"github.com/buildpack/pack"
	"github.com/spf13/cobra"
)

func main() {
	wd, _ := os.Getwd()

	var appDir, stackName string
	var publish bool
	buildCommand := &cobra.Command{
		Use:  "build [IMAGE NAME]",
		Args: cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			repoName := args[0]
			return pack.Build(appDir, stackName, repoName, os.Getenv("PACK_HOST_MACHINE_IP"), publish)
		},
	}
	buildCommand.Flags().StringVarP(&appDir, "path", "p", wd, "path to app dir")
	buildCommand.Flags().StringVarP(&stackName, "stack", "s", "packs/v3", "stack")
	buildCommand.Flags().BoolVarP(&publish, "publish", "r", false, "publish to registry")

	rootCmd := &cobra.Command{Use: "pack"}
	rootCmd.AddCommand(buildCommand)
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
