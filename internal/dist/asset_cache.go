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

	pubcfg "github.com/buildpacks/pack/config"

	"github.com/buildpacks/imgutil"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/pkg/errors"

	"github.com/buildpacks/pack/internal/blob"
	"github.com/buildpacks/pack/internal/layer"
	"github.com/buildpacks/pack/pkg/archive"
)

const AssetCacheLayersLabel = "io.buildpacks.asset.layers"
const AssetHashAlgorithm = "sha256"

type BlobAssetPair struct {
	Blob  blob.Blob
	Asset Asset
}

type AssetCacheImage struct {
	imgutil.Image
	layerWriterFactory *layer.WriterFactory
	assetsAndBlobs     []BlobAssetPair
}

func NewAssetCacheImage(img imgutil.Image, layerWriterFactory *layer.WriterFactory) (*AssetCacheImage, error) {
	return &AssetCacheImage{
		Image:              img,
		layerWriterFactory: layerWriterFactory,
	}, nil
}

// TODO -Dan- Document that ordering of pairs matters here,
//   if two elements share a sha256 value, the last one will win
//   asset.Sha256 values should be unique.
func (a *AssetCacheImage) AddAssetLayers(pairs ...BlobAssetPair) {
	for _, p := range pairs {
		switch p.Blob {
		case nil:
			continue
		default:
			a.assetsAndBlobs = append(a.assetsAndBlobs, p)
		}
	}
}

func (a *AssetCacheImage) Save(additionalNames ...string) error {
	metadata := AssetMap{}
	tmpDir, err := ioutil.TempDir("", "create-asset-scratch")
	if err != nil {
		return err
	}
	imgOS, err := a.OS()
	if err != nil {
		return errors.Wrap(err, "unable to get asset cache image os")
	}

	if imgOS == pubcfg.WindowsOS {
		err = AddWindowsShimBaseLayer(a, tmpDir)
		if err != nil {
			return errors.Wrap(err, "unable to write windows base layer")
		}
	}

	for _, pair := range a.assetsAndBlobs {
		diffID, err := a.addBlobLayer(pair.Blob, pair.Asset.Sha256, filepath.Join(tmpDir, pair.Asset.Sha256))
		if err != nil {
			return errors.Wrapf(err, "unable to add asset blob %q to layer", pair.Asset.Sha256)
		}
		metadata[pair.Asset.Sha256] = pair.Asset.ToAssetValue(diffID)
	}

	assetLabelBuf := bytes.NewBuffer(nil)
	err = json.NewEncoder(assetLabelBuf).Encode(metadata)
	if err != nil {
		return errors.Wrap(err, "unable to encode asset cache metadata as json")
	}

	err = a.SetLabel(AssetCacheLayersLabel, assetLabelBuf.String())
	if err != nil {
		return errors.Wrap(err, "unable to set asset cache label")
	}

	return a.Image.Save(additionalNames...)
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
	tw := a.layerWriterFactory.NewWriter(w)
	if err = toAssetTar(tw, blobSha256, b); err != nil {
		return "", err
	}
	if err = a.AddLayer(layerPath); err != nil {
		return "", err
	}

	return fmt.Sprintf("%s:%x", AssetHashAlgorithm, hash.Sum(nil)), nil
}

func toAssetTar(tw archive.TarWriter, blobSha string, blob Blob) error {
	ts := archive.NormalizedDateTime

	if err := tw.WriteHeader(&tar.Header{
		Typeflag: tar.TypeDir,
		Name:     path.Join("/cnb"),
		Mode:     0755,
		ModTime:  ts,
	}); err != nil {
		return errors.Wrapf(err, "writing asset-cache /cnb dir header")
	}

	if err := tw.WriteHeader(&tar.Header{
		Typeflag: tar.TypeDir,
		Name:     path.Join("/cnb", "assets"),
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
