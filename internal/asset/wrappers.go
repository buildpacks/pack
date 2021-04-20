package asset

import (
	"io"
)

type wrapper struct {
	Func func(string) (io.ReadCloser, error)
	Arg  string
}

func (w wrapper) Open() (io.ReadCloser, error) {
	return w.Func(w.Arg)
}
