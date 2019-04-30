package pack

import (
	"crypto/sha256"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"path/filepath"

	"github.com/pkg/errors"

	"github.com/buildpack/pack/archive"
)

type Logger interface {
	Verbose(format string, a ...interface{})
}

type Downloader struct {
	logger   Logger
	cacheDir string
}

func NewDownloader(logger Logger, cacheDir string) *Downloader {
	return &Downloader{
		logger:   logger,
		cacheDir: cacheDir,
	}
}

func (d *Downloader) Download(uri string) (string, error) {
	url, err := url.Parse(uri)
	if err != nil {
		return "", err
	}

	switch url.Scheme {
	case "", "file":
		return d.handleFile(url)
	case "http", "https":
		return d.handleHTTP(uri)
	default:
		return "", fmt.Errorf("unsupported protocol in URI %q", uri)
	}
}

func (d *Downloader) handleFile(bpURL *url.URL) (string, error) {
	path := bpURL.Path

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

func (d *Downloader) handleHTTP(uri string) (string, error) {
	bpCache := filepath.Join(d.cacheDir, fmt.Sprintf("%x", sha256.Sum256([]byte(uri))))
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

	reader, etag, err := d.downloadAsStream(uri, etag)
	if err != nil {
		return "", errors.Wrapf(err, "failed to download from %q", uri)
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

func (d *Downloader) downloadAsStream(uri string, etag string) (io.ReadCloser, string, error) {
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
		d.logger.Verbose("Downloading from %q\n", uri)
		return resp.Body, resp.Header.Get("Etag"), nil
	}

	if resp.StatusCode == 304 {
		d.logger.Verbose("Using cached version of %q\n", uri)
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
