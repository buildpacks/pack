//go:build windows
// +build windows

package sshdialer_test

import (
	"gopkg.in/natefinch/npipe.v2"
	"net"
	"os/user"
	"strings"

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

	// See https://github.com/hectane/go-acl/issues/1
	if err != nil && err.Error() != "The operation completed successfully." {
		panic(err)
	}
}

func listen(addr string) (net.Listener, error) {
	if strings.Contains(addr, "\\pipe\\") {
		return npipe.Listen(addr)
	} else {
		return net.Listen("unix", addr)
	}
}
