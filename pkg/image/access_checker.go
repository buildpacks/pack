package image

import (
	"github.com/buildpacks/imgutil/remote"
	"github.com/google/go-containerregistry/pkg/authn"

	"github.com/buildpacks/pack/pkg/logging"
)

type Checker struct {
	logger   logging.Logger
	keychain authn.Keychain
}

func NewAccessChecker(logger logging.Logger, keychain authn.Keychain) *Checker {
	checker := &Checker{
		logger:   logger,
		keychain: keychain,
	}

	if checker.keychain == nil {
		checker.keychain = authn.DefaultKeychain
	}

	return checker
}

func (c *Checker) Check(repo string, publish bool) bool {
	if !publish {
		// nop checker, we are running against the daemon
		return true
	}

	img, err := remote.NewImage(repo, c.keychain)
	if err != nil {
		return false
	}

	if ok, err := img.CheckReadAccess(); ok {
		c.logger.Debugf("CheckReadAccess succeeded for the run image %s", repo)
		return true
	} else {
		c.logger.Debugf("CheckReadAccess failed for the run image %s, error: %s", repo, err.Error())
		return false
	}
}
