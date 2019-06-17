package lifecycle

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"regexp"

	"github.com/Masterminds/semver"
	"github.com/pkg/errors"
)

const (
	DefaultLifecycleVersion = "0.2.1"
)

//go:generate mockgen -package mocks -destination mocks/downloader.go github.com/buildpack/pack/lifecycle Downloader

type Downloader interface {
	Download(uri string) (string, error)
}

type Fetcher struct {
	downloader Downloader
}

func NewFetcher(downloader Downloader) *Fetcher {
	return &Fetcher{downloader: downloader}
}

func (f *Fetcher) Fetch(version *semver.Version, uri string) (Metadata, error) {
	if version == nil && uri == "" {
		version = semver.MustParse(DefaultLifecycleVersion)
	}

	if uri == "" {
		uri = fmt.Sprintf("https://github.com/buildpack/lifecycle/releases/download/v%s/lifecycle-v%s+linux.x86-64.tgz", version.String(), version.String())
	}

	path, err := f.downloader.Download(uri)
	if err != nil {
		return Metadata{}, errors.Wrapf(err, "retrieving lifecycle from %s", uri)
	}

	err = validateTarEntries(
		path,
		"detector",
		"restorer",
		"analyzer",
		"builder",
		"exporter",
		"cacher",
		"launcher",
	)
	if err != nil {
		return Metadata{}, errors.Wrapf(err, "invalid lifecycle")
	}

	return Metadata{Version: version, Path: path}, nil
}

func validateTarEntries(tarPath string, entryPath ...string) error {
	var (
		tarFile    *os.File
		gzipReader *gzip.Reader
		fhFinal    io.Reader
		err        error
	)

	tarFile, err = os.Open(tarPath)
	fhFinal = tarFile
	if err != nil {
		return errors.Wrapf(err, "failed to open tar '%s' for validation", tarPath)
	}
	defer tarFile.Close()

	if filepath.Ext(tarPath) == ".tgz" {
		gzipReader, err = gzip.NewReader(tarFile)
		fhFinal = gzipReader
		if err != nil {
			return errors.Wrap(err, "failed to create gzip reader")
		}

		defer gzipReader.Close()
	}

	regex := regexp.MustCompile(`^[^/]+/([^/]+)$`)
	headers := map[string]bool{}
	tr := tar.NewReader(fhFinal)
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

	for _, p := range entryPath {
		_, found := headers[p]
		if !found {
			return fmt.Errorf("did not find '%s' in tar", p)
		}
	}

	return nil
}
