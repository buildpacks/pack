package image

import (
	"io"
	"time"
)

type Image interface {
	Name() string
	Rename(name string)
	Digest() (string, error)
	Label(string) (string, error)
	SetLabel(string, string) error
	Env(key string) (string, error)
	SetEnv(string, string) error
	SetEntrypoint(...string) error
	SetCmd(...string) error
	Rebase(string, Image) error
	AddLayer(path string) error
	ReuseLayer(sha string) error
	TopLayer() (string, error)
	Save() (string, error)
	Found() (bool, error)
	GetLayer(string) (io.ReadCloser, error)
	Delete() error
	CreatedAt() (time.Time, error)
}
