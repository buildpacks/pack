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
	"regexp"

	"github.com/pkg/errors"

	"github.com/buildpack/pack/internal/paths"
	"github.com/buildpack/pack/logging"
)

type Downloader struct {
	logger   logging.Logger
	cacheDir string
}

var schemeRegexp = regexp.MustCompile(`^.+://.*`)

func NewDownloader(logger logging.Logger, cacheDir string) *Downloader {
	return &Downloader{
		logger:   logger,
		cacheDir: cacheDir,
	}
}

func (d *Downloader) Download(pathOrUri string) (string, error) {
	hasScheme := schemeRegexp.MatchString(pathOrUri)
	if hasScheme {
		parsedUrl, err := url.Parse(pathOrUri)
		if err != nil {
			return "", err
		}

		switch parsedUrl.Scheme {
		case "file":
			return paths.UriToFilePath(pathOrUri)
		case "http", "https":
			return d.handleHTTP(pathOrUri)
		default:
			return "", fmt.Errorf("unsupported protocol '%s' in URI %q", parsedUrl.Scheme, pathOrUri)
		}
	} else {
		return d.handleFile(pathOrUri)
	}
}

func (d *Downloader) handleFile(path string) (string, error) {
	var (
		err error
	)

	if path, err = filepath.Abs(path); err != nil {
		return "", nil
	}

	return path, nil
}

func (d *Downloader) handleHTTP(uri string) (string, error) {
	if err := os.MkdirAll(d.cacheDir, 0744); err != nil {
		return "", err
	}

	cachePath := filepath.Join(d.cacheDir, fmt.Sprintf("%x", sha256.Sum256([]byte(uri))))
	tgzFile := cachePath + ".tgz"

	etagFile := cachePath + ".etag"
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
		return tgzFile, nil
	}
	defer reader.Close()

	fh, err := os.Create(tgzFile)
	if err != nil {
		return "", err
	}
	defer fh.Close()

	_, err = io.Copy(fh, reader)
	if err != nil {
		return "", err
	}

	if err = ioutil.WriteFile(etagFile, []byte(etag), 0744); err != nil {
		return "", err
	}

	return tgzFile, nil
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
		d.logger.Debugf("Downloading from %q", uri)
		return resp.Body, resp.Header.Get("Etag"), nil
	}

	if resp.StatusCode == 304 {
		d.logger.Debugf("Using cached version of %q", uri)
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
