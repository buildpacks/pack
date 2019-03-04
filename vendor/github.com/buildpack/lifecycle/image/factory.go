package image

import (
	"io"
	"io/ioutil"

	"github.com/docker/docker/client"
	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/pkg/errors"

	"github.com/buildpack/lifecycle/fs"
	"github.com/buildpack/lifecycle/image/auth"
)

type Factory struct {
	Docker   *client.Client
	FS       *fs.FS
	Keychain authn.Keychain
	Out      io.Writer
}

func NewFactory(ops ...func(*Factory)) (*Factory, error) {
	f := &Factory{
		FS:       &fs.FS{},
		Out:      ioutil.Discard,
		Keychain: authn.DefaultKeychain,
	}

	var err error
	f.Docker, err = newDocker()
	if err != nil {
		return nil, err
	}
	for _, op := range ops {
		op(f)
	}

	return f, nil
}

func WithEnvKeychain(factory *Factory) {
	factory.Keychain = authn.NewMultiKeychain(&auth.EnvKeychain{}, factory.Keychain)
}

func WithLegacyEnvKeychain(factory *Factory) {
	factory.Keychain = authn.NewMultiKeychain(&auth.LegacyEnvKeychain{}, factory.Keychain)
}

func WithOutWriter(w io.Writer) func(factory *Factory) {
	return func(factory *Factory) {
		factory.Out = w
	}
}

func newDocker() (*client.Client, error) {
	docker, err := client.NewClientWithOpts(client.FromEnv, client.WithVersion("1.38"))
	if err != nil {
		return nil, errors.Wrap(err, "new docker client")
	}
	return docker, nil
}
