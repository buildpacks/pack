package buildpack

import "strings"

type BuildpackTOML struct {
	Buildpack Buildpack
	Stacks    []Stack
}

type Buildpack struct {
	BuildpackInfo
	Path   string
	Stacks []Stack
	Order  Order
}

type Order []Group

type Group struct {
	Group []BuildpackInfo
}

type BuildpackInfo struct {
	ID      string `toml:"id" json:"id"`
	Version string `toml:"version" json:"version"`
}

type Stack struct {
	ID string
}

func (b *Buildpack) EscapedID() string {
	return strings.Replace(b.ID, "/", "_", -1)
}

func (b *Buildpack) SupportsStack(stackID string) bool {
	for _, stack := range b.Stacks {
		if stack.ID == stackID {
			return true
		}
	}
	return false
}
