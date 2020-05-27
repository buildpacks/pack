package main

import (
	"os"

	"github.com/heroku/color"

	"github.com/buildpacks/pack"
	"github.com/buildpacks/pack/cli"
	"github.com/buildpacks/pack/internal/commands"
	clilogger "github.com/buildpacks/pack/internal/logging"
)

func main() {
	// create logger with defaults
	logger := clilogger.NewLogWithWriters(color.Stdout(), color.Stderr())

	rootCmd, err := cli.NewPackCommand(logger)
	if err != nil {
		logger.Error(err.Error())
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
