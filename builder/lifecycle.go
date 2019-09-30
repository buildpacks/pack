package builder

import (
	"archive/tar"
	"fmt"
	"io"
	"path"
	"regexp"

	"github.com/BurntSushi/toml"
	"github.com/pkg/errors"

	"github.com/buildpack/pack/api"
	"github.com/buildpack/pack/dist"
	"github.com/buildpack/pack/internal/archive"
)

const (
	AssumedLifecycleVersion   = "0.3.0"
	AssumedPlatformAPIVersion = "0.1"

	DefaultLifecycleVersion    = "0.4.0"
	DefaultBuildpackAPIVersion = "0.2"
	DefaultPlatformAPIVersion  = "0.1"
)

var (
	lifecycleBinaries = []string{
		"detector",
		"restorer",
		"analyzer",
		"builder",
		"exporter",
		"cacher",
		"launcher",
	}
)

type Blob interface {
	Open() (io.ReadCloser, error)
}

//go:generate mockgen -package testmocks -destination testmocks/lifecycle.go github.com/buildpack/pack/builder Lifecycle
type Lifecycle interface {
	Blob
	Descriptor() LifecycleDescriptor
}

type LifecycleDescriptor struct {
	Info LifecycleInfo `toml:"lifecycle"`
	API  LifecycleAPI  `toml:"api"`
}

type LifecycleInfo struct {
	Version *Version `toml:"version" json:"version"`
}

type LifecycleAPI struct {
	BuildpackVersion *api.Version `toml:"buildpack" json:"buildpack"`
	PlatformVersion  *api.Version `toml:"platform" json:"platform"`
}

type lifecycle struct {
	descriptor LifecycleDescriptor
	Blob
}

func NewLifecycle(blob Blob) (Lifecycle, error) {
	var err error

	br, err := blob.Open()
	if err != nil {
		return nil, errors.Wrap(err, "open lifecycle blob")
	}
	defer br.Close()

	var descriptor LifecycleDescriptor
	_, buf, err := archive.ReadTarEntry(br, "lifecycle.toml")

	//TODO: make lifecycle descriptor required after v0.4.0 release [https://github.com/buildpack/pack/issues/267]
	if err != nil && errors.Cause(err) == archive.ErrEntryNotExist {
		return &lifecycle{
			Blob: blob,
			descriptor: LifecycleDescriptor{
				Info: LifecycleInfo{
					Version: VersionMustParse(AssumedLifecycleVersion),
				},
				API: LifecycleAPI{
					BuildpackVersion: api.MustParse(dist.AssumedBuildpackAPIVersion),
					PlatformVersion:  api.MustParse(AssumedPlatformAPIVersion),
				},
			},
		}, nil
	} else if err != nil {
		return nil, errors.Wrap(err, "decode lifecycle descriptor")
	}
	_, err = toml.Decode(string(buf), &descriptor)
	if err != nil {
		return nil, errors.Wrap(err, "decoding descriptor")
	}

	lifecycle := &lifecycle{Blob: blob, descriptor: descriptor}

	if err = lifecycle.validateBinaries(); err != nil {
		return nil, errors.Wrap(err, "validating binaries")
	}

	return lifecycle, nil
}

func (l *lifecycle) Descriptor() LifecycleDescriptor {
	return l.descriptor
}

func (l *lifecycle) validateBinaries() error {
	rc, err := l.Open()
	if err != nil {
		return errors.Wrap(err, "create lifecycle blob reader")
	}
	defer rc.Close()
	regex := regexp.MustCompile(`^[^/]+/([^/]+)$`)
	headers := map[string]bool{}
	tr := tar.NewReader(rc)
	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return errors.Wrap(err, "failed to get next tar entry")
		}

		pathMatches := regex.FindStringSubmatch(path.Clean(header.Name))
		if pathMatches != nil {
			headers[pathMatches[1]] = true
		}
	}
	for _, p := range lifecycleBinaries {
		_, found := headers[p]
		if !found {
			return fmt.Errorf("did not find '%s' in tar", p)
		}
	}
	return nil
}
