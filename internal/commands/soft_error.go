package commands

type SoftError struct {
}

func NewSoftError() SoftError {
	return SoftError{}
}

func (se SoftError) Error() string {
	return ""
}
