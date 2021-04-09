package asset

import (
	"archive/tar"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path"
	"path/filepath"

	"github.com/pkg/errors"

	"github.com/buildpacks/pack/internal/dist"
	"github.com/buildpacks/pack/pkg/archive"
)

//go:generate mockgen -package testmocks -destination testmocks/layer_writer.go github.com/buildpacks/pack/internal/asset LayerWriter
type LayerWriter interface {
	Open() error
	Close() error
	Write(w Writable) error
	AddAssetBlobs(aBlobs ...Blob)
	AssetMetadata() dist.AssetMap
}

type layerWriter struct {
	tmpDir        string
	blobs         []Blob
	metadata      dist.AssetMap
	writerFactory archive.TarWriterFactory
}

func NewLayerWriter(writerFactory archive.TarWriterFactory) LayerWriter {
	return &layerWriter{
		blobs:         []Blob{},
		metadata:      dist.AssetMap{},
		writerFactory: writerFactory,
	}
}

type Writable interface {
	AddLayerWithDiffID(path, diffID string) error
	SetLabel(string, string) error
}

// TODO -Dan- give open some options
func (lw *layerWriter) Open() error {
	if lw.tmpDir != "" {
		return errors.New("unable to open writer: writer already open")
	}

	tmpDir, err := ioutil.TempDir("", "writer-workspace")
	if err != nil {
		return err
	}

	lw.tmpDir = tmpDir
	return nil
}

func (lw *layerWriter) Close() error {
	if lw.tmpDir == "" {
		return errors.New("unable to close writer: writer is not open")
	}
	err := os.RemoveAll(lw.tmpDir)
	if err != nil {
		return err
	}

	lw.tmpDir = ""
	return nil
}

func (lw *layerWriter) Write(w Writable) error {
	if lw.tmpDir == "" {
		return errors.New("layerWriter must be opened before writing")
	}

	for _, aBlob := range lw.blobs {
		aBlob := aBlob // force copy operation
		// TODO -Dan- handle cases of 128+ layers on image.
		layerFileName := filepath.Join(lw.tmpDir, aBlob.AssetDescriptor().Sha256)
		descriptor := aBlob.AssetDescriptor()
		// TODO -Dan- handle multiple assets per layer
		assetLayerReader := archive.GenerateTarWithWriter(func(tw archive.TarWriter) error {
			return toAssetTar(tw, descriptor.Sha256, aBlob)
		}, lw.writerFactory)

		layerDiffID, err := createAssetLayerFile(layerFileName, assetLayerReader)
		if err != nil {
			return errors.Wrapf(err, "unable to create asset layer file")
		}
		err = w.AddLayerWithDiffID(layerFileName, "sha256:"+layerDiffID)
		if err != nil {
			return errors.Wrapf(err, "unable to write layer")
		}

		m, ok := lw.metadata[descriptor.Sha256]
		if !ok {
			// TODO -Dan- handle this case
			return fmt.Errorf("unknown sha256 asset value %s", descriptor.Sha256)
		}
		m.LayerDiffID = "sha256:" + layerDiffID
		lw.metadata[descriptor.Sha256] = m
	}

	return dist.SetLabel(w, LayersLabel, lw.metadata)
}

func (lw *layerWriter) AddAssetBlobs(aBlobs ...Blob) {
	lw.blobs = append(lw.blobs, aBlobs...)
	for _, b := range aBlobs {
		// TODO -Dan- handle over-writes
		descriptor := b.AssetDescriptor()
		assetMetadata := descriptor
		lw.metadata[descriptor.Sha256] = assetMetadata.ToAssetValue("")
	}
}

func (lw *layerWriter) AssetMetadata() dist.AssetMap {
	return lw.metadata
}

func createAssetLayerFile(layerFileName string, assetLayer io.ReadCloser) (string, error) {
	layerFile, err := os.OpenFile(layerFileName, os.O_RDWR|os.O_TRUNC|os.O_CREATE, 0644)
	if err != nil {
		return "", err
	}
	defer layerFile.Close()

	hasher := sha256.New()
	teeWriter := io.MultiWriter(layerFile, hasher)

	_, err = io.Copy(teeWriter, assetLayer)
	if err != nil {
		return "", err
	}

	sha256Hash := hex.EncodeToString(hasher.Sum(nil))
	return sha256Hash, nil
}

func toAssetTar(tw archive.TarWriter, blobSha string, blob dist.Blob) error {
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
