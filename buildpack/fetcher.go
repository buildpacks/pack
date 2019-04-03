package buildpack

import (
	"crypto/sha256"
	"fmt"
	"github.com/buildpack/pack/archive"
	"github.com/pkg/errors"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
)

type Logger interface {
	Verbose(format string, a ...interface{})
}

type Fetcher struct {
	Logger   Logger
	CacheDir string
}

func NewFetcher(logger Logger, cacheDir string) *Fetcher {
	return &Fetcher{
		Logger:   logger,
		CacheDir: filepath.Join(cacheDir, "dl-cache"),
	}
}

func (f *Fetcher) FetchBuildpack(localSearchPath string, bp Buildpack) (out Buildpack, err error) {
	out = Buildpack{
		ID:      bp.ID,
		URI:     bp.URI,
		Latest:  bp.Latest,
		Version: bp.Version,
	}

	bpURL, err := url.Parse(bp.URI)
	if err != nil {
		return out, err
	}

	switch bpURL.Scheme {
	case "", "file":
		out.Dir, err = f.handleFile(localSearchPath, bpURL)
	case "http", "https":
		out.Dir, err = f.handleHTTP(bp)
	default:
		return out, fmt.Errorf("unsupported protocol in URI %q", bp.URI)
	}

	return out, err
}

func (f *Fetcher) handleFile(localSearchPath string, bpURL *url.URL) (string, error) {
	path := bpURL.Path

	if !bpURL.IsAbs() && !filepath.IsAbs(path) {
		path = filepath.Join(localSearchPath, path)
	}

	if filepath.Ext(path) != ".tgz" {
		return path, nil
	}

	file, err := os.Open(path)
	if err != nil {
		return "", errors.Wrapf(err, "could not open file to untar: %q", path)
	}
	defer file.Close()

	tmpDir, err := ioutil.TempDir("", "")
	if err != nil {
		return "", fmt.Errorf(`failed to create temporary directory: %s`, err)
	}

	if err = archive.ExtractTarGZ(file, tmpDir); err != nil {
		return "", err
	}

	return tmpDir, nil
}

func (f *Fetcher) handleHTTP(bp Buildpack) (string, error) {
	bpCache := filepath.Join(f.CacheDir, fmt.Sprintf("%x", sha256.Sum256([]byte(bp.URI))))
	if err := os.MkdirAll(bpCache, 0744); err != nil {
		return "", err
	}

	etagFile := bpCache + ".etag"
	etagExists, err := fileExists(etagFile)
	if err != nil {
		return "", err
	}

	etag := ""
	if etagExists {
		bytes, err := ioutil.ReadFile(etagFile)
		if err != nil {
			return "", err
		}
		etag = string(bytes)
	}

	reader, etag, err := f.downloadAsStream(bp.URI, etag)
	if err != nil {
		return "", errors.Wrapf(err, "failed to download from %q", bp.URI)
	} else if reader == nil {
		return bpCache, nil
	}
	defer reader.Close()

	if err = archive.ExtractTarGZ(reader, bpCache); err != nil {
		return "", err
	}

	if err = ioutil.WriteFile(etagFile, []byte(etag), 0744); err != nil {
		return "", err
	}

	return bpCache, nil
}

func (f *Fetcher) downloadAsStream(uri string, etag string) (io.ReadCloser, string, error) {
	req, err := http.NewRequest("GET", uri, nil)
	if err != nil {
		return nil, "", err
	}

	if etag != "" {
		req.Header.Set("If-None-Match", etag)
	}

	resp, err := (&http.Client{}).Do(req)
	if err != nil {
		return nil, "", err
	}

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		f.Logger.Verbose("Downloading from %q\n", uri)
		return resp.Body, resp.Header.Get("Etag"), nil
	}

	if resp.StatusCode == 304 {
		f.Logger.Verbose("Using cached version of %q\n", uri)
		return nil, etag, nil
	}

	return nil, "", fmt.Errorf("could not download from %q, code http status %d", uri, resp.StatusCode)
}

func fileExists(file string) (bool, error) {
	_, err := os.Stat(file)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}
