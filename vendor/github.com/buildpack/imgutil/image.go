package imgutil

import (
	"fmt"
	"io"
	"time"
)

type SaveDiagnostic struct {
	ImageName string
	Cause     error
}

type SaveError struct {
	Errors []SaveDiagnostic
}

func (e SaveError) Error() string {
	var failed []string
	for _, d := range e.Errors {
		failed = append(failed, d.ImageName)
	}
	return fmt.Sprintf("failed to write image to the following tags: %+v", failed)
}

type Image interface {
	Name() string
	Rename(name string)
	Label(string) (string, error)
	SetLabel(string, string) error
	Env(key string) (string, error)
	SetEnv(string, string) error
	SetEntrypoint(...string) error
	SetWorkingDir(string) error
	SetCmd(...string) error
	Rebase(string, Image) error
	AddLayer(path string) error
	ReuseLayer(sha string) error
	TopLayer() (string, error)
	// Save saves the image as `Name()` and any additional names provided to this method.
	Save(additionalNames ...string) error
	// Found tells whether the image exists in the repository by `Name()`.
	Found() bool
	GetLayer(sha string) (io.ReadCloser, error)
	Delete() error
	CreatedAt() (time.Time, error)
	Identifier() (Identifier, error)
}

type Identifier fmt.Stringer
