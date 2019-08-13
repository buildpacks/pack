package buildpack

import (
	"io/ioutil"
	"path/filepath"

	"github.com/BurntSushi/toml"
	"github.com/pkg/errors"

	"github.com/buildpack/pack/internal/archive"
)

//go:generate mockgen -package mocks -destination mocks/downloader.go github.com/buildpack/pack/lifecycle Downloader

type Downloader interface {
	Download(uri string) (string, error)
}

type buildpackTOML struct {
	Buildpack struct {
		ID      string `toml:"id"`
		Version string `toml:"version"`
	} `toml:"buildpack"`
	Order  Order   `toml:"order"`
	Stacks []Stack `toml:"stacks"`
}

type Fetcher struct {
	downloader Downloader
}

func NewFetcher(downloader Downloader) *Fetcher {
	return &Fetcher{downloader: downloader}
}

func (f *Fetcher) FetchBuildpack(uri string) (Buildpack, error) {
	downloadedPath, err := f.downloader.Download(uri)
	if err != nil {
		return Buildpack{}, errors.Wrap(err, "fetching buildpack")
	}

	data, err := readTOML(downloadedPath)
	if err != nil {
		return Buildpack{}, err
	}

	return Buildpack{
		BuildpackInfo: BuildpackInfo{
			ID:      data.Buildpack.ID,
			Version: data.Buildpack.Version,
		},
		Path:   downloadedPath,
		Order:  data.Order,
		Stacks: data.Stacks,
	}, err
}

func readTOML(path string) (buildpackTOML, error) {
	var (
		buf []byte
		err error
	)
	if filepath.Ext(path) == ".tgz" {
		_, buf, err = archive.ReadTarEntry(path, "./buildpack.toml", "buildpack.toml", "/buildpack.toml")
	} else {
		buf, err = ioutil.ReadFile(filepath.Join(path, "buildpack.toml"))
	}

	if err != nil {
		return buildpackTOML{}, err
	}

	bpTOML := buildpackTOML{}
	_, err = toml.Decode(string(buf), &bpTOML)
	if err != nil {
		return buildpackTOML{}, errors.Wrapf(err, "reading buildpack.toml from path %s", path)
	}
	return bpTOML, nil
}
