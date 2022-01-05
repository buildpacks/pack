//go:build !windows
// +build !windows

package sshdialer_test

import (
	"net"
	"os"
)

func fixupPrivateKeyMod(path string) {
	err := os.Chmod(path, 0400)
	if err != nil {
		panic(err)
	}
}

func listen(addr string) (net.Listener, error) {
	return net.Listen("unix", addr)
}
