package image

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"strings"

	"github.com/buildpacks/imgutil/remote"

	"github.com/buildpacks/imgutil"
	"github.com/buildpacks/imgutil/local"
	"github.com/buildpacks/lifecycle/auth"
	"github.com/docker/docker/api/types"
	"github.com/docker/docker/client"
	"github.com/docker/docker/pkg/jsonmessage"
	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/pkg/errors"

	"github.com/buildpacks/pack/config"
	ilogging "github.com/buildpacks/pack/internal/logging"
	"github.com/buildpacks/pack/internal/style"
	"github.com/buildpacks/pack/logging"
)

type Fetcher struct {
	docker client.CommonAPIClient
	logger logging.Logger
}

func NewFetcher(logger logging.Logger, docker client.CommonAPIClient) *Fetcher {
	return &Fetcher{
		logger: logger,
		docker: docker,
	}
}

var ErrNotFound = errors.New("not found")

func (f *Fetcher) Fetch(ctx context.Context, name string, daemon bool, pullPolicy config.PullPolicy) (imgutil.Image, error) {
	if !daemon {
		return f.fetchRemoteImage(name)
	}

	switch pullPolicy {
	case config.PullNever:
		img, err := f.fetchDaemonImage(name)
		return img, err
	case config.PullIfNotPresent:
		img, err := f.fetchDaemonImage(name)
		if err == nil || !errors.Is(err, ErrNotFound) {
			return img, err
		}
	}

	f.logger.Debugf("Pulling image %s", style.Symbol(name))
	err := f.pullImage(ctx, name)
	if err != nil && !errors.Is(err, ErrNotFound) {
		return nil, err
	}

	return f.fetchDaemonImage(name)
}

func (f *Fetcher) fetchDaemonImage(name string) (imgutil.Image, error) {
	image, err := local.NewImage(name, f.docker, local.FromBaseImage(name))
	if err != nil {
		return nil, err
	}

	if !image.Found() {
		return nil, errors.Wrapf(ErrNotFound, "image %s does not exist on the daemon", style.Symbol(name))
	}

	return image, nil
}

func (f *Fetcher) fetchRemoteImage(name string) (imgutil.Image, error) {
	image, err := remote.NewImage(name, authn.DefaultKeychain, remote.FromBaseImage(name))
	if err != nil {
		return nil, err
	}

	if !image.Found() {
		return nil, errors.Wrapf(ErrNotFound, "image %s does not exist in registry", style.Symbol(name))
	}

	return image, nil
}

func (f *Fetcher) pullImage(ctx context.Context, imageID string) error {
	regAuth, err := registryAuth(imageID)
	if err != nil {
		return err
	}

	rc, err := f.docker.ImagePull(ctx, imageID, types.ImagePullOptions{RegistryAuth: regAuth})
	if err != nil {
		if client.IsErrNotFound(err) {
			return errors.Wrapf(ErrNotFound, "image %s does not exist on the daemon", style.Symbol(imageID))
		}

		return err
	}

	writer := logging.GetWriterForLevel(f.logger, logging.InfoLevel)
	termFd, isTerm := ilogging.IsTerminal(writer)

	err = jsonmessage.DisplayJSONMessagesStream(rc, &colorizedWriter{writer}, termFd, isTerm, nil)
	if err != nil {
		return err
	}

	return rc.Close()
}

func registryAuth(ref string) (string, error) {
	_, a, err := auth.ReferenceForRepoName(authn.DefaultKeychain, ref)
	if err != nil {
		return "", errors.Wrapf(err, "resolve auth for ref %s", ref)
	}
	authConfig, err := a.Authorization()
	if err != nil {
		return "", err
	}

	dataJSON, err := json.Marshal(authConfig)
	if err != nil {
		return "", err
	}

	return base64.StdEncoding.EncodeToString(dataJSON), nil
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
		msg = strings.ReplaceAll(msg, pattern, colorize(pattern))
	}
	return w.writer.Write([]byte(msg))
}
