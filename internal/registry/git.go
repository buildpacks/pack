package registry

import (
	"bytes"
	"os"
	"text/template"

	"github.com/pkg/errors"
)

func CreateGitCommit(b Buildpack, username string, registryCache Cache) error {
	_, err := os.Stat(registryCache.Root)
	if err != nil {
		if os.IsNotExist(err) {
			err = registryCache.CreateCache()
			if err != nil {
				return errors.Wrap(err, "creating registry cache")
			}
		}
	}

	commitTemplate, err := template.New("buildpack").Parse(GitCommitTemplate)
	if err != nil {
		return err
	}

	var commit bytes.Buffer
	if err := commitTemplate.Execute(&commit, b); err != nil {
		return err
	}

	if err := registryCache.CreateCommit(b, username, commit.String()); err != nil {
		return err
	}

	return nil
}
