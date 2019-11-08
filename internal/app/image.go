package app

import "github.com/buildpack/pack/logging"

type Image struct {
	RepoName string
	Logger   logging.Logger
}
