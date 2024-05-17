package storage

import (
	"fmt"

	"github.com/buildpacks/pack/internal/style"
)

type MountType string

const (
	VOLUME MountType = "volume"
	BIND MountType = "bind"
)

func ParseMount(mount string) (m MountType) {
	if m = MountType(mount); m != VOLUME && m != BIND {
		fmt.Printf("using default mount: %s", style.Symbol(string(VOLUME)))
		return VOLUME
	}

	return m
}