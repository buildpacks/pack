package main

import (
	"os"

	"github.com/apex/log"

	"github.com/buildpacks/pack/cmd"

	"github.com/buildpacks/pack"
	"github.com/buildpacks/pack/internal/commands"
)

func main() {
	rootCmd, err := cmd.NewPackCommand()
	if err != nil {
		log.Error(err.Error())
		os.Exit(1)
	}

	ctx := commands.CreateCancellableContext()
	if err := rootCmd.ExecuteContext(ctx); err != nil {
		if _, isSoftError := err.(pack.SoftError); isSoftError {
			os.Exit(2)
		}
		os.Exit(1)
	}
}
