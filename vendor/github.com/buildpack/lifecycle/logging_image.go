package lifecycle

import (
	"github.com/buildpack/lifecycle/image"
	"github.com/pkg/errors"
	"log"
)

type loggingImage struct {
	Out   *log.Logger
	image image.Image
}

func (li *loggingImage) AddLayer(identifier, sha, tar string) error {
	li.Out.Printf("adding layer '%s' with diffID '%s'\n", identifier, sha)
	if err := li.image.AddLayer(tar); err != nil {
		return errors.Wrapf(err, "add %s layer", identifier)
	}
	return nil
}

func (li *loggingImage) ReuseLayer(identifier, sha string) error {
	li.Out.Printf("reusing layer '%s' with diffID '%s'\n", identifier, sha)
	if err := li.image.ReuseLayer(sha); err != nil {
		return errors.Wrapf(err, "reuse %s layer", identifier)
	}
	return nil
}

func (li *loggingImage) SetLabel(k string, v string) error {
	li.Out.Printf("setting metadata label '%s'\n", k)
	return li.image.SetLabel(k, v)
}

func (li *loggingImage) SetEnv(k string, v string) error {
	li.Out.Printf("setting env var '%s=%s'\n", k, v)
	return li.image.SetEnv(k, v)
}

func (li *loggingImage) SetEntrypoint(entryPoint string) error {
	li.Out.Printf("setting entrypoint '%s'\n", entryPoint)
	return li.image.SetEntrypoint(entryPoint)
}

func (li *loggingImage) SetEmptyCmd() error {
	li.Out.Println("setting empty cmd")
	return li.image.SetCmd()
}

func (li *loggingImage) Save() (string, error) {
	li.Out.Println("writing image")
	sha, err := li.image.Save()
	li.Out.Printf("\n*** Image: %s@%s\n", li.image.Name(), sha)
	return sha, err
}
