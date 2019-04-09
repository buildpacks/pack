package image

import (
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"strings"

	img "github.com/buildpack/lifecycle/image"
	"github.com/buildpack/lifecycle/image/auth"
	"github.com/docker/docker/api/types"
	"github.com/docker/docker/client"
	"github.com/docker/docker/pkg/jsonmessage"
	"github.com/docker/docker/pkg/term"
	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/pkg/errors"

	"github.com/buildpack/pack/logging"
	"github.com/buildpack/pack/style"
)

//go:generate mockgen -package mocks -destination mocks/image_factory.go github.com/buildpack/pack ImageFactory
type ImageFactory interface {
	NewLocal(string) (img.Image, error)
	NewRemote(string) (img.Image, error)
}

type Fetcher struct {
	docker  *client.Client
	factory *img.Factory
	logger  *logging.Logger
}

func NewFetcher(logger *logging.Logger, docker *client.Client) (*Fetcher, error) {
	factory, err := img.NewFactory()
	if err != nil {
		return nil, err
	}

	return &Fetcher{
		logger:  logger,
		docker:  docker,
		factory: factory,
	}, nil
}

var ErrNotFound = errors.New("not found")

func (f *Fetcher) Fetch(ctx context.Context, name string, daemon, pull bool) (i img.Image, err error) {
	i, err = f.factory.NewRemote(name)
	if err != nil {
		return nil, err
	}

	remoteFound, err := i.Found()
	if err != nil {
		return nil, err
	}

	if daemon {
		if remoteFound && pull {
			f.logger.Verbose("Pulling image %s", style.Symbol(name))
			if err := f.pullImage(ctx, name); err != nil {
				return nil, err
			}
		}
		return f.fetchDaemonImage(name)
	}

	if !remoteFound {
		return nil, errors.Wrapf(ErrNotFound, "image '%s' does not exist in registry", name)
	}

	return i, nil
}

func (f *Fetcher) fetchDaemonImage(name string) (img.Image, error) {
	i, err := f.factory.NewLocal(name)
	if err != nil {
		return nil, err
	}

	found, err := i.Found()
	if err != nil {
		return nil, err
	}

	if !found {
		return nil, errors.Wrapf(ErrNotFound, "image '%s' does not exist on the daemon", name)
	}
	return i, nil
}

func (f *Fetcher) pullImage(ctx context.Context, imageID string) error {
	auth, err := registryAuth(imageID)
	if err != nil {
		return err
	}
	rc, err := f.docker.ImagePull(ctx, imageID, types.ImagePullOptions{
		RegistryAuth: auth,
	})
	if err != nil {
		return err
	}
	termFd, isTerm := term.GetFdInfo(f.logger.RawVerboseWriter())
	err = jsonmessage.DisplayJSONMessagesStream(rc, &colorizedWriter{f.logger.RawVerboseWriter()}, termFd, isTerm, nil)
	if err != nil {
		return err
	}

	return rc.Close()
}

func registryAuth(ref string) (string, error) {
	var regAuth string
	_, a, err := auth.ReferenceForRepoName(authn.DefaultKeychain, ref)
	if err != nil {
		return "", errors.Wrapf(err, "resolve auth for ref %s", ref)
	}
	authHeader, err := a.Authorization()
	if err != nil {
		return "", err
	}
	if strings.HasPrefix(authHeader, "Basic ") {
		encoded := strings.TrimPrefix(authHeader, "Basic ")
		decoded, _ := base64.StdEncoding.DecodeString(encoded)
		parts := strings.SplitN(string(decoded), ":", 2)
		regAuth = base64.StdEncoding.EncodeToString(
			[]byte(fmt.Sprintf(
				`{"username": "%s", "password": "%s"}`,
				parts[0],
				parts[1],
			)),
		)
	}
	return regAuth, nil
}

type colorizedWriter struct {
	writer io.Writer
}

type colorFunc = func(string, ...interface{}) string

func (w *colorizedWriter) Write(p []byte) (n int, err error) {
	msg := string(p)
	colorizers := map[string]colorFunc{
		"Waiting":           style.Waiting,
		"Pulling fs layer":  style.Waiting,
		"Downloading":       style.Working,
		"Download complete": style.Working,
		"Extracting":        style.Working,
		"Pull complete":     style.Complete,
		"Already exists":    style.Complete,
		"=":                 style.ProgressBar,
		">":                 style.ProgressBar,
	}
	for pattern, colorize := range colorizers {
		msg = strings.Replace(msg, pattern, colorize(pattern), -1)
	}
	return w.writer.Write([]byte(msg))
}
