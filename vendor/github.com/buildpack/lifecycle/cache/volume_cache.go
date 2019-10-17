package cache

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"math/rand"
	"os"
	"path/filepath"

	"github.com/pkg/errors"
)

type VolumeCache struct {
	committed    bool
	dir          string
	backupDir    string
	stagingDir   string
	committedDir string
}

func NewVolumeCache(dir string) (*VolumeCache, error) {
	if _, err := os.Stat(dir); err != nil {
		return nil, err
	}

	c := &VolumeCache{
		dir:          dir,
		backupDir:    filepath.Join(dir, "committed-backup"),
		stagingDir:   filepath.Join(dir, "staging"),
		committedDir: filepath.Join(dir, "committed"),
	}

	if err := c.setupStagingDir(); err != nil {
		return nil, errors.Wrapf(err, "initializing staging directory '%s'", c.stagingDir)
	}

	if err := os.RemoveAll(c.backupDir); err != nil {
		return nil, errors.Wrapf(err, "removing backup directory '%s'", c.backupDir)
	}

	if err := os.MkdirAll(c.committedDir, 0777); err != nil {
		return nil, errors.Wrapf(err, "creating committed directory '%s'", c.committedDir)
	}

	return c, nil
}

func (c *VolumeCache) Name() string {
	return c.dir
}

func (c *VolumeCache) SetMetadata(metadata Metadata) error {
	if c.committed {
		return errCacheCommitted
	}
	metadataPath := filepath.Join(c.stagingDir, MetadataLabel)
	file, err := os.Create(metadataPath)
	if err != nil {
		return errors.Wrapf(err, "creating metadata file '%s'", metadataPath)
	}
	defer file.Close()

	if err := json.NewEncoder(file).Encode(metadata); err != nil {
		return errors.Wrap(err, "marshalling metadata")
	}

	return nil
}

func (c *VolumeCache) RetrieveMetadata() (Metadata, error) {
	metadataPath := filepath.Join(c.committedDir, MetadataLabel)
	file, err := os.Open(metadataPath)
	if err != nil {
		if os.IsNotExist(err) {
			return Metadata{}, nil
		}
		return Metadata{}, errors.Wrapf(err, "opening metadata file '%s'", metadataPath)
	}
	defer file.Close()

	metadata := Metadata{}
	if json.NewDecoder(file).Decode(&metadata) != nil {
		return Metadata{}, nil
	}
	return metadata, nil
}

func (c *VolumeCache) AddLayerFile(sha string, tarPath string) error {
	if c.committed {
		return errCacheCommitted
	}
	if err := copyFile(tarPath, filepath.Join(c.stagingDir, sha+".tar")); err != nil {
		return errors.Wrapf(err, "caching layer (%s)", sha)
	}
	return nil
}

func (c *VolumeCache) AddLayer(rc io.ReadCloser) error {
	if c.committed {
		return errCacheCommitted
	}

	tarFile := filepath.Join(c.stagingDir, randString(10)+".tar")
	fh, err := os.Create(tarFile)
	if err != nil {
		return errors.Wrapf(err, "create layer file in cache")
	}

	hasher := sha256.New()
	mw := io.MultiWriter(hasher, fh)
	if _, err := io.Copy(mw, rc); err != nil {
		fh.Close()
		return errors.Wrap(err, "copying layer to tar file")
	}
	sha := hex.EncodeToString(hasher.Sum(make([]byte, 0, hasher.Size())))
	if err := fh.Close(); err != nil {
		return errors.Wrapf(err, "closing layer file (layer sha: %s)", sha)
	}
	if err := os.Rename(tarFile, filepath.Join(c.stagingDir, "sha256:"+sha+".tar")); err != nil {
		return errors.Wrapf(err, "renaming layer file (layer sha: %s)", sha)
	}
	return nil
}

func (c *VolumeCache) ReuseLayer(sha string) error {
	if c.committed {
		return errCacheCommitted
	}
	if err := os.Link(filepath.Join(c.committedDir, sha+".tar"), filepath.Join(c.stagingDir, sha+".tar")); err != nil {
		return errors.Wrapf(err, "reusing layer (%s)", sha)
	}
	return nil
}

func (c *VolumeCache) RetrieveLayer(sha string) (io.ReadCloser, error) {
	path, err := c.RetrieveLayerFile(sha)
	if err != nil {
		return nil, err
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, errors.Wrapf(err, "opening layer with SHA '%s'", sha)
	}
	return file, nil
}

func (c *VolumeCache) HasLayer(sha string) (bool, error) {
	if _, err := os.Stat(filepath.Join(c.committedDir, sha+".tar")); err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, errors.Wrapf(err, "retrieving layer with SHA '%s'", sha)
	}
	return true, nil
}

func (c *VolumeCache) RetrieveLayerFile(sha string) (string, error) {
	path := filepath.Join(c.committedDir, sha+".tar")
	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			return "", errors.Wrapf(err, "layer with SHA '%s' not found", sha)
		}
		return "", errors.Wrapf(err, "retrieving layer with SHA '%s'", sha)
	}
	return path, nil
}

func (c *VolumeCache) Commit() error {
	if c.committed {
		return errCacheCommitted
	}
	c.committed = true
	if err := os.Rename(c.committedDir, c.backupDir); err != nil {
		return errors.Wrap(err, "backing up cache")
	}
	defer os.RemoveAll(c.backupDir)

	if err1 := os.Rename(c.stagingDir, c.committedDir); err1 != nil {
		if err2 := os.Rename(c.backupDir, c.committedDir); err2 != nil {
			return errors.Wrap(err2, "rolling back cache")
		}
		return errors.Wrap(err1, "committing cache")
	}

	return nil
}

func (c *VolumeCache) setupStagingDir() error {
	if err := os.RemoveAll(c.stagingDir); err != nil {
		return err
	}
	return os.MkdirAll(c.stagingDir, 0777)
}

func copyFile(from, to string) error {
	in, err := os.Open(from)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.Create(to)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, in)

	return err
}

func randString(n int) string {
	b := make([]byte, n)
	for i := range b {
		b[i] = 'a' + byte(rand.Intn(26))
	}
	return string(b)
}
