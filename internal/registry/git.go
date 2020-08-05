package registry

import (
	"bytes"
	"text/template"
)

func CreateGitCommit(b Buildpack, registryCache Cache) error {
	if err := registryCache.Refresh(); err != nil {
		return err
	}

	commitTemplate, err := template.New("buildpack").Parse(GitCommitTemplate)
	if err != nil {
		return err
	}

	var commit bytes.Buffer
	if err := commitTemplate.Execute(&commit, b); err != nil {
		return err
	}

	if err := registryCache.CreateCommit(b, commit.String()); err != nil {
		return err
	}

	return nil
}
