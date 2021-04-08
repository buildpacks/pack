package asset

import (
	"io"
)

type Wrapper struct {
	Func func(string) (io.ReadCloser, error)
	Arg string
}

func (w Wrapper) Open() (io.ReadCloser, error) {
	return w.Func(w.Arg)
}