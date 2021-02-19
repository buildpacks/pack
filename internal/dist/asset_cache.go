package dist

import (
	"archive/tar"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path"
	"path/filepath"

	"github.com/buildpacks/imgutil"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/pkg/errors"

	"github.com/buildpacks/pack/internal/blob"
	"github.com/buildpacks/pack/pkg/archive"
)

const AssetCacheLayersLabel = "io.buildpacks.asset.layers"
const AssetHashAlgorithm = "sha256"

type AssetCacheImage struct {
	Map BlobMap
	img imgutil.Image
}

func NewAssetCacheImage(img imgutil.Image, m BlobMap) AssetCacheImage {
	return AssetCacheImage{
		img: img,
		Map: m,
	}
}

func (a *AssetCacheImage) Save() error {
	tmpDir, err := ioutil.TempDir("", "create-asset-scratch")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmpDir)

	metadata := AssetMap{}
	for _, key := range a.Map.Keys() {
		assetBlobPair := a.Map[key]
		if assetBlobPair.Blob == nil {
			continue
		}
		diffID, err := a.addBlobLayer(assetBlobPair.Blob, key, filepath.Join(tmpDir, key))
		if err != nil {
			return err
		}
		assetBlobPair.AssetVal.LayerDiffID = diffID
		metadata[key] = assetBlobPair.AssetVal
	}

	assetLabelBuf := bytes.NewBuffer(nil)
	err = json.NewEncoder(assetLabelBuf).Encode(metadata)
	if err != nil {
		return err
	}

	err = a.img.SetLabel(AssetCacheLayersLabel, assetLabelBuf.String())
	if err != nil {
		return err
	}

	return a.img.Save()
}

func (a *AssetCacheImage) addBlobLayer(b blob.Blob, blobSha256 string, layerPath string) (diffID string, err error) {
	dstTar, err := os.OpenFile(layerPath, os.O_RDWR|os.O_CREATE|os.O_TRUNC, os.ModePerm)
	if err != nil {
		return "", errors.Wrapf(err, "unable to open asset layer %q for writing", layerPath)
	}
	defer dstTar.Close()

	hash, err := v1.Hasher(AssetHashAlgorithm)
	if err != nil {
		return "", err
	}

	w := io.MultiWriter(dstTar, hash)
	tw := tar.NewWriter(w)
	if err = toAssetTar(tw, blobSha256, b); err != nil {
		return "", err
	}
	if err = a.img.AddLayer(layerPath); err != nil {
		return "", err
	}

	return fmt.Sprintf("%s:%x", AssetHashAlgorithm, hash.Sum(nil)), nil
}

func toAssetTar(tw archive.TarWriter, blobSha string, blob Blob) error {
	ts := archive.NormalizedDateTime

	if err := tw.WriteHeader(&tar.Header{
		Typeflag: tar.TypeDir,
		Name:     path.Join("cnb"),
		Mode:     0755,
		ModTime:  ts,
	}); err != nil {
		return errors.Wrapf(err, "writing asset-cache /cnb dir header")
	}

	if err := tw.WriteHeader(&tar.Header{
		Typeflag: tar.TypeDir,
		Name:     path.Join("cnb", "assets"),
		Mode:     0755,
		ModTime:  ts,
	}); err != nil {
		return errors.Wrapf(err, "writing asset-cache /cnb/asset dir header")
	}

	buf := bytes.NewBuffer(nil)
	rc, err := blob.Open()
	if err != nil {
		return errors.Wrapf(err, "unable to open blob for asset %q", blobSha)
	}
	defer rc.Close()

	_, err = io.Copy(buf, rc)
	if err != nil {
		return errors.Wrap(err, "unable to copy blob contents to buffer")
	}

	if err := tw.WriteHeader(&tar.Header{
		Typeflag: tar.TypeReg,
		Name:     path.Join("/cnb", "assets", blobSha),
		Mode:     0755,
		Size:     int64(buf.Len()),
		ModTime:  ts,
	}); err != nil {
		return errors.Wrapf(err, "writing asset-cache /cnb/asset/%s file", blobSha)
	}

	_, err = tw.Write(buf.Bytes())
	return err
}
