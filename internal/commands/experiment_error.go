package commands

import (
	"github.com/buildpacks/pack/internal/style"
	"github.com/buildpacks/pack/logging"
)

type ExperimentError struct {
	msg string
}

func NewExperimentError(msg string) ExperimentError {
	return ExperimentError{msg}
}

func (ee ExperimentError) Error() string {
	return ee.msg
}

func (ee ExperimentError) Tip(logger logging.Logger, configPath string) {
	logging.Tip(logger, "To enable experimental features, add %s to %s.", style.Symbol("experimental = true"), style.Symbol(configPath))
}
