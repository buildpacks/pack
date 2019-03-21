package commands

type SoftError struct {
}

func IsSoftError(err error) bool {
	_, isSoft := err.(SoftError)
	return isSoft
}

func MakeSoftError() SoftError {
	return SoftError{}
}

func (se SoftError) Error() string {
	return ""
}
