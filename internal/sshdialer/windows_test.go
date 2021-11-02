//go:build windows
// +build windows

package sshdialer_test

import (
	"os/user"

	"github.com/hectane/go-acl"
)

func fixupPrivateKeyMod(path string) {
	usr, err := user.Current()
	if err != nil {
		panic(err)
	}
	mode := uint32(0400)
	err = acl.Apply(path,
		true,
		false,
		acl.GrantName(((mode&0700)<<23)|((mode&0200)<<9), usr.Name))
	if err != nil {
		panic(err)
	}
}
