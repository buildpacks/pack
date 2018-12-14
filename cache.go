package pack

import (
	"crypto/md5"
	"fmt"

	"github.com/google/go-containerregistry/pkg/name"
	"github.com/pkg/errors"
)

func CacheVolume(repoName string) (string, error) {
	ref, err := name.ParseReference(repoName, name.WeakValidation)
	if err != nil {
		return "", errors.Wrap(err, "bad image identifier")
	}
	cacheVolume := fmt.Sprintf("pack-cache-%x", md5.Sum([]byte(ref.String())))
	return cacheVolume, nil
}