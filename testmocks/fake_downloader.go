package testmocks

import (
	"context"

	"github.com/buildpacks/pack/internal/blob"
)

type FakeDownloader struct {
	err error
}

func NewFakeDownloader(err error) FakeDownloader {
	return FakeDownloader{err: err}
}

func (f FakeDownloader) Download(_ context.Context, _ string, _ ...blob.DownloadOption) (blob.Blob, error) {
	return nil, f.err
}
