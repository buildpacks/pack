package cache

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"

	"github.com/pkg/errors"
)

type VolumeCache struct {
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

func (c *VolumeCache) AddLayer(identifier string, sha string, tarPath string) error {
	if err := copyFile(tarPath, filepath.Join(c.stagingDir, sha+".tar")); err != nil {
		return errors.Wrapf(err, "caching layer '%s' (%s)", identifier, sha)
	}
	return nil
}

func (c *VolumeCache) ReuseLayer(identifier string, sha string) error {
	if err := copyFile(filepath.Join(c.committedDir, sha+".tar"), filepath.Join(c.stagingDir, sha+".tar")); err != nil {
		return errors.Wrapf(err, "reusing layer '%s' (%s)", identifier, sha)
	}
	return nil
}

func (c *VolumeCache) RetrieveLayer(sha string) (io.ReadCloser, error) {
	file, err := os.Open(filepath.Join(c.committedDir, sha+".tar"))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, errors.Wrapf(err, "layer with SHA '%s' not found", sha)
		}
		return nil, errors.Wrapf(err, "retrieving layer with SHA '%s'", sha)
	}
	return file, nil
}

func (c *VolumeCache) Commit() error {
	if err := os.Rename(c.committedDir, c.backupDir); err != nil {
		return errors.Wrap(err, "backing up cache")
	}
	defer os.RemoveAll(c.backupDir)

	if err := os.Rename(c.stagingDir, c.committedDir); err != nil {
		if err := os.Rename(c.backupDir, c.committedDir); err != nil {
			return errors.Wrap(err, "rolling back cache")
		}
		return nil
	}

	return c.setupStagingDir()
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
