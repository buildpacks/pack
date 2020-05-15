package commands

import (
	"os"

	"github.com/buildpacks/pack/internal/style"
	"github.com/buildpacks/pack/logging"
)

type ExperimentError struct {
	msg string
}

func IsExperimentError(err error) bool {
	_, isExperiment := err.(ExperimentError)
	return isExperiment
}

func MakeExperimentError(msg string) ExperimentError {
	return ExperimentError{msg}
}

func (ee ExperimentError) Error() string {
	return ee.msg
}

func (ee ExperimentError) Tip(logger logging.Logger) {
	configPath, isExist := os.LookupEnv("CONFIG_PATH")
	if !isExist {
		configPath = "CONFIG_PATH"
	}
	logging.Tip(logger, "To enable experimental features, add %s to %s.", style.Symbol("experimental = true"), style.Symbol(configPath))
}
