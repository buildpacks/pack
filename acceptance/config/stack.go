//go:build acceptance
// +build acceptance

package config

type Stack struct {
	RunImage       RunImage
	BuildImageName string
}

type RunImage struct {
	Name       string
	MirrorName string
}
