package buildpack

import (
	"path/filepath"

	"github.com/BurntSushi/toml"
	"github.com/pkg/errors"
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
	Stacks []Stack `toml:"stacks"`
}

type Fetcher struct {
	downloader Downloader
}

func NewFetcher(downloader Downloader) *Fetcher {
	return &Fetcher{downloader: downloader}
}

func (f *Fetcher) FetchBuildpack(uri string) (Buildpack, error) {
	dir, err := f.downloader.Download(uri)
	if err != nil {
		return Buildpack{}, errors.Wrap(err, "fetching buildpack")
	}

	data, err := readTOML(filepath.Join(dir, "buildpack.toml"))
	if err != nil {
		return Buildpack{}, err
	}

	return Buildpack{
		Dir:     dir,
		ID:      data.Buildpack.ID,
		Version: data.Buildpack.Version,
		Stacks:  data.Stacks,
	}, err
}

func readTOML(path string) (buildpackTOML, error) {
	data := buildpackTOML{}
	_, err := toml.DecodeFile(path, &data)
	if err != nil {
		return buildpackTOML{}, errors.Wrapf(err, "reading buildpack.toml from path %s", path)
	}
	return data, nil
}
