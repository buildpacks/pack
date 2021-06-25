package blob

import (
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"path/filepath"

	v1 "github.com/google/go-containerregistry/pkg/v1"

	"github.com/mitchellh/ioprogress"
	"github.com/pkg/errors"

	"github.com/buildpacks/pack/internal/paths"
	"github.com/buildpacks/pack/internal/style"
	"github.com/buildpacks/pack/logging"
)

const (
	cacheDirPrefix = "c"
	cacheVersion   = "2"
)

type downloader struct {
	logger       logging.Logger
	baseCacheDir string
}

type downloadSettings struct {
	blobOptions      []Option
	validationSha256 string
}

func NewDownloader(logger logging.Logger, baseCacheDir string) downloader { //nolint:golint,gosimple
	return downloader{
		logger:       logger,
		baseCacheDir: baseCacheDir,
	}
}

type DownloadOption func(*downloadSettings)

func (d downloader) Download(ctx context.Context, pathOrURI string, options ...DownloadOption) (Blob, error) {
	settings := &downloadSettings{}
	for _, option := range options {
		option(settings)
	}

	if paths.IsURI(pathOrURI) {
		parsedURL, err := url.Parse(pathOrURI)
		if err != nil {
			return nil, errors.Wrapf(err, "parsing path/uri %s", style.Symbol(pathOrURI))
		}

		var path string
		switch parsedURL.Scheme {
		case "file":
			path, err = paths.URIToFilePath(pathOrURI)
		case "http", "https":
			path, err = d.handleHTTP(ctx, pathOrURI)
		default:
			err = fmt.Errorf("unsupported protocol %s in URI %s", style.Symbol(parsedURL.Scheme), style.Symbol(pathOrURI))
		}
		if err != nil {
			return nil, err
		}

		if err := validateBlobSha(NewBlob(path, settings.blobOptions...), settings.validationSha256); err != nil {
			return nil, err
		}
		return NewBlob(path, settings.blobOptions...), nil
	}

	path := d.handleFile(pathOrURI)

	if err := validateBlobSha(NewBlob(path, settings.blobOptions...), settings.validationSha256); err != nil {
		return nil, err
	}
	return NewBlob(path, settings.blobOptions...), nil
}

//
// Download Options
//

func RawDownload(s *downloadSettings) {
	s.blobOptions = append(s.blobOptions, RawOption)
}

func ValidateDownload(sha256 string) DownloadOption {
	return func(s *downloadSettings) {
		s.validationSha256 = fmt.Sprintf("sha256:%s", sha256)
	}
}

func (d downloader) handleFile(path string) string {
	path, err := filepath.Abs(path)
	if err != nil {
		return ""
	}

	return path
}

func (d downloader) handleHTTP(ctx context.Context, uri string) (string, error) {
	cacheDir := d.versionedCacheDir()

	if err := os.MkdirAll(cacheDir, 0750); err != nil {
		return "", err
	}

	cachePath := filepath.Join(cacheDir, fmt.Sprintf("%x", sha256.Sum256([]byte(uri))))

	etagFile := cachePath + ".etag"
	etagExists, err := fileExists(etagFile)
	if err != nil {
		return "", err
	}

	etag := ""
	if etagExists {
		bytes, err := ioutil.ReadFile(filepath.Clean(etagFile))
		if err != nil {
			return "", err
		}
		etag = string(bytes)
	}

	reader, etag, err := d.downloadAsStream(ctx, uri, etag)
	if err != nil {
		return "", err
	} else if reader == nil {
		return cachePath, nil
	}
	defer reader.Close()

	fh, err := os.Create(cachePath)
	if err != nil {
		return "", errors.Wrapf(err, "create cache path %s", style.Symbol(cachePath))
	}
	defer fh.Close()

	_, err = io.Copy(fh, reader)
	if err != nil {
		return "", errors.Wrap(err, "writing cache")
	}

	if err = ioutil.WriteFile(etagFile, []byte(etag), 0744); err != nil {
		return "", errors.Wrap(err, "writing etag")
	}

	return cachePath, nil
}

func (d downloader) downloadAsStream(ctx context.Context, uri string, etag string) (io.ReadCloser, string, error) {
	req, err := http.NewRequest("GET", uri, nil)
	if err != nil {
		return nil, "", err
	}
	req = req.WithContext(ctx)

	if etag != "" {
		req.Header.Set("If-None-Match", etag)
	}

	resp, err := (&http.Client{}).Do(req) //nolint:bodyclose
	if err != nil {
		return nil, "", err
	}

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		d.logger.Infof("Downloading from %s", style.Symbol(uri))
		return withProgress(logging.GetWriterForLevel(d.logger, logging.InfoLevel), resp.Body, resp.ContentLength), resp.Header.Get("Etag"), nil
	}

	if resp.StatusCode == 304 {
		d.logger.Debugf("Using cached version of %s", style.Symbol(uri))
		return nil, etag, nil
	}

	return nil, "", fmt.Errorf(
		"could not download from %s, code http status %s",
		style.Symbol(uri), style.SymbolF("%d", resp.StatusCode),
	)
}

func withProgress(writer io.Writer, rc io.ReadCloser, length int64) io.ReadCloser {
	return &progressReader{
		Closer: rc,
		Reader: &ioprogress.Reader{
			Reader:   rc,
			Size:     length,
			DrawFunc: ioprogress.DrawTerminalf(writer, ioprogress.DrawTextFormatBytes),
		},
	}
}

type progressReader struct {
	*ioprogress.Reader
	io.Closer
}

func (d downloader) versionedCacheDir() string {
	return filepath.Join(d.baseCacheDir, cacheDirPrefix+cacheVersion)
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

func validateBlobSha(b Blob, expectedSha256 string) error {
	if expectedSha256 == "" {
		return nil
	}
	r, err := b.Open()
	if err != nil {
		return err
	}
	defer r.Close()
	h, _, err := v1.SHA256(r)
	if err != nil {
		return err
	}

	if expectedSha256 != h.String() {
		return fmt.Errorf("sha256 validation failed, expected %q, got %q", expectedSha256, h.String())
	}

	return nil
}
