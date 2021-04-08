package fakes

import (
	"archive/tar"
	"bytes"
	"github.com/buildpacks/pack/internal/dist"
	"github.com/buildpacks/pack/pkg/archive"
	"github.com/pkg/errors"
	"io"
	"io/ioutil"
	"path"
	"strings"
)



type FakeBlob struct {
	Contents string
}

func NewFakeBlob(contents string) FakeBlob {
	return FakeBlob{
		Contents: contents,
	}
}

func (f FakeBlob) Open() (io.ReadCloser, error) {
	return ioutil.NopCloser(strings.NewReader(f.Contents)), nil
}

type FakeAssetBlob struct {
	Asset dist.Asset
	Contents string
}

func NewFakeAssetBlob(contents string, asset dist.Asset) FakeAssetBlob {
	return FakeAssetBlob{
		Contents: contents,
		Asset: asset,
	}
}

func NewFakeAssetBlobTar(rawContents string, asset dist.Asset, factory archive.TarWriterFactory) (FakeAssetBlob, error) {
	buf := bytes.NewBuffer(nil)
	tw := factory.NewWriter(buf)
	ts := archive.NormalizedDateTime

	if err := tw.WriteHeader(&tar.Header{
		Typeflag: tar.TypeDir,
		Name:     path.Join("/cnb"),
		Mode:     0755,
		ModTime:  ts,
	}); err != nil {
		return FakeAssetBlob{}, errors.Wrapf(err, "writing /cnb directory in fakeAsset")
	}

	if err := tw.WriteHeader(&tar.Header{
		Typeflag: tar.TypeDir,
		Name:     path.Join("/cnb", "assets"),
		Mode:     0755,
		ModTime:  ts,
	}); err != nil {
		return FakeAssetBlob{}, errors.Wrapf(err, "writing /cnb/assets directory in fakeAsset")
	}

	if err := tw.WriteHeader(&tar.Header{
		Typeflag: tar.TypeReg,
		Name:     path.Join("/cnb", "assets", asset.Sha256),
		Mode:     0755,
		Size:     int64(len(rawContents)),
		ModTime:  ts,
	}); err != nil {
		return FakeAssetBlob{}, errors.New("writing /cnb/assets/<sha> file in fakeAsset")
	}

	_, err := tw.Write([]byte(rawContents))
	if err != nil {
		return FakeAssetBlob{}, errors.New("writing asset file contents in fakeAsset")
	}

	return NewFakeAssetBlob(buf.String(), asset), nil

}

func (f FakeAssetBlob) Open() (io.ReadCloser, error) {
	return ioutil.NopCloser(strings.NewReader(f.Contents)), nil
}

func (f FakeAssetBlob) Size() int64 {
	return int64(len(f.Contents))
}

func (f FakeAssetBlob) AssetDescriptor() dist.Asset {
	return f.Asset
}
