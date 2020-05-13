package commands

type ExperimentError struct {
	msg string
}

const tip = "Tip: To enable experimental features, add `experimental = true` to {{CONFIG_PATH}}."

func MakeExperimentError(msg string) ExperimentError {
	return ExperimentError{msg}
}

func (ee ExperimentError) Error() string {
	return ee.msg + "\n" + tip
}
