package lifecycle

import (
	"fmt"
	"io/ioutil"
	"path/filepath"

	"github.com/Masterminds/semver"
	"github.com/pkg/errors"

	"github.com/buildpack/pack/style"
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

	downloadDir, err := f.downloader.Download(uri)
	if err != nil {
		return Metadata{}, errors.Wrapf(err, "retrieving lifecycle from %s", uri)
	}

	dir, err := getLifecycleParentDir(downloadDir)
	if err != nil {
		return Metadata{}, errors.Wrapf(err, "invalid lifecycle")
	}

	return Metadata{Version: version, Dir: dir}, nil
}

func getLifecycleParentDir(root string) (string, error) {
	fis, err := ioutil.ReadDir(root)
	if err != nil {
		return "", err
	}
	if len(fis) == 1 && fis[0].IsDir() {
		return getLifecycleParentDir(filepath.Join(root, fis[0].Name()))
	}

	bins := map[string]bool{
		"detector": false,
		"restorer": false,
		"analyzer": false,
		"builder":  false,
		"exporter": false,
		"cacher":   false,
		"launcher": false,
	}

	for _, fi := range fis {
		if _, ok := bins[fi.Name()]; ok {
			bins[fi.Name()] = true
		}
	}

	for bin, found := range bins {
		if !found {
			return "", fmt.Errorf("missing required lifecycle binary %s", style.Symbol(bin))
		}
	}
	return root, nil
}
