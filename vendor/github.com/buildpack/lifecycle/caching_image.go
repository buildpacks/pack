package lifecycle

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"

	"github.com/buildpack/imgutil"
	"github.com/pkg/errors"

	"github.com/buildpack/lifecycle/cache"
)

type cachingImage struct {
	imgutil.Image
	cache *cache.VolumeCache
}

func NewCachingImage(image imgutil.Image, cache *cache.VolumeCache) imgutil.Image {
	return &cachingImage{
		Image: image,
		cache: cache,
	}
}

func (c *cachingImage) AddLayer(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return errors.Wrap(err, "opening layer file")
	}
	defer f.Close()

	hasher := sha256.New()
	if _, err := io.Copy(hasher, f); err != nil {
		return errors.Wrap(err, "hashing layer")
	}
	sha := hex.EncodeToString(hasher.Sum(make([]byte, 0, hasher.Size())))

	if err := c.cache.AddLayerFile("sha256:"+sha, path); err != nil {
		return err
	}

	return c.Image.AddLayer(path)
}

func (c *cachingImage) ReuseLayer(sha string) error {
	found, err := c.cache.HasLayer(sha)
	if err != nil {
		return err
	}

	if found {
		if err := c.cache.ReuseLayer(sha); err != nil {
			return err
		}
		path, err := c.cache.RetrieveLayerFile(sha)
		if err != nil {
			return err
		}
		return c.Image.AddLayer(path)
	}

	if err := c.Image.ReuseLayer(sha); err != nil {
		return err
	}
	rc, err := c.Image.GetLayer(sha)
	if err != nil {
		return err
	}
	return c.cache.AddLayer(rc)
}

func (c *cachingImage) GetLayer(sha string) (io.ReadCloser, error) {
	if found, err := c.cache.HasLayer(sha); err != nil {
		return nil, fmt.Errorf("cache no layer with sha '%s'", sha)
	} else if found {
		return c.cache.RetrieveLayer(sha)
	}
	return c.Image.GetLayer(sha)
}

func (c *cachingImage) Save(additionalNames ...string) error {
	err := c.Image.Save(additionalNames...)

	if saveSucceededFor(c.Name(), err) {
		if err := c.cache.Commit(); err != nil {
			return errors.Wrap(err, "failed to commit cache")
		}
	}
	return err
}

func saveSucceededFor(imageName string, err error) bool {
	if err == nil {
		return true
	}

	if saveErr, isSaveErr := err.(imgutil.SaveError); isSaveErr {
		for _, d := range saveErr.Errors {
			if d.ImageName == imageName {
				return false
			}
		}
		return true
	}
	return false
}
