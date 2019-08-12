package buildpack

import "strings"

type BuildpackTOML struct {
	Buildpack Buildpack
	Stacks    []Stack
}

type Buildpack struct {
	ID      string
	Path    string
	Version string
	Stacks  []Stack
	Order   Order
}

type Order []Group

type Group struct {
	Group []BuildpackRef
}

type BuildpackRef struct {
	ID      string
	Version string
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
