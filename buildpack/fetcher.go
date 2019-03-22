package buildpack

import (
	"crypto/sha256"
	"fmt"
	"github.com/buildpack/pack/archive"
	"github.com/buildpack/pack/config"
	"github.com/buildpack/pack/logging"
	"github.com/pkg/errors"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
)

// TODO : test this by itself, currently it is tested in create_builder_test.go
// TODO : attempt to use this during build with the --buildpack flag to get tar.gz buildpacks
// TODO : think of a better name for this construct
type Fetcher struct {
	Config *config.Config
	Logger *logging.Logger
}

func NewFetcher(cfg *config.Config, logger *logging.Logger) *Fetcher {
	return &Fetcher{
		Config: cfg,
		Logger: logger,
	}
}

func (f *Fetcher) FetchBuildpack(builderDir string, b Buildpack) (Buildpack, error) {
	var dir string

	asURL, err := url.Parse(b.URI)
	if err != nil {
		return Buildpack{}, err
	}

	switch asURL.Scheme {
	case "", // This is the only way to support relative filepaths
		"file": // URIs with file:// protocol force the use of absolute paths. Host=localhost may be implied with file:///

		path := asURL.Path

		if !asURL.IsAbs() && !filepath.IsAbs(path) {
			path = filepath.Join(builderDir, path)
		}

		if filepath.Ext(path) == ".tgz" {
			file, err := os.Open(path)
			if err != nil {
				return Buildpack{}, errors.Wrapf(err, "could not open file to untar: %q", path)
			}
			defer file.Close()
			tmpDir, err := ioutil.TempDir("", fmt.Sprintf("create-builder-%s-", b.EscapedID()))
			if err != nil {
				return Buildpack{}, fmt.Errorf(`failed to create temporary directory: %s`, err)
			}
			if err = archive.ExtractTarGZ(file, tmpDir); err != nil {
				return Buildpack{}, err
			}
			dir = tmpDir
		} else {
			dir = path
		}
	case "http", "https":
		uriDigest := fmt.Sprintf("%x", sha256.Sum256([]byte(b.URI)))
		cachedDir := filepath.Join(f.Config.Path(), "dl-cache", uriDigest)
		_, err := os.Stat(cachedDir)
		if os.IsNotExist(err) {
			if err = os.MkdirAll(cachedDir, 0744); err != nil {
				return Buildpack{}, err
			}
		}
		etagFile := cachedDir + ".etag"
		bytes, err := ioutil.ReadFile(etagFile)
		etag := ""
		if err == nil {
			etag = string(bytes)
		}

		reader, etag, err := f.downloadAsStream(b.URI, etag)
		if err != nil {
			return Buildpack{}, errors.Wrapf(err, "failed to download from %q", b.URI)
		} else if reader == nil {
			// can use cached content
			dir = cachedDir
			break
		}
		defer reader.Close()

		if err = archive.ExtractTarGZ(reader, cachedDir); err != nil {
			return Buildpack{}, err
		}

		if err = ioutil.WriteFile(etagFile, []byte(etag), 0744); err != nil {
			return Buildpack{}, err
		}

		dir = cachedDir
	default:
		return Buildpack{}, fmt.Errorf("unsupported protocol in URI %q", b.URI)
	}

	return Buildpack{
		ID:     b.ID,
		Latest: b.Latest,
		Dir:    dir,
	}, nil
}

func (f *Fetcher) downloadAsStream(uri string, etag string) (io.ReadCloser, string, error) {
	c := http.Client{}
	req, err := http.NewRequest("GET", uri, nil)
	if err != nil {
		return nil, "", err
	}
	if etag != "" {
		req.Header.Set("If-None-Match", etag)
	}
	if resp, err := c.Do(req); err != nil {
		return nil, "", err
	} else {
		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			f.Logger.Verbose("Downloading from %q\n", uri)
			return resp.Body, resp.Header.Get("Etag"), nil
		} else if resp.StatusCode == 304 {
			f.Logger.Verbose("Using cached version of %q\n", uri)
			return nil, etag, nil
		} else {
			return nil, "", fmt.Errorf("could not download from %q, code http status %d", uri, resp.StatusCode)
		}
	}
}
