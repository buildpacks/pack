//go:build windows
// +build windows

package sshdialer_test

import (
	"errors"
	"net"
	"strings"

	"gopkg.in/natefinch/npipe.v2"
)

func fixupPrivateKeyMod(path string) {
}

func listen(addr string) (net.Listener, error) {
	if strings.Contains(addr, "\\pipe\\") {
		return npipe.Listen(addr)
	}
	return net.Listen("unix", addr)
}

func isErrClosed(err error) bool {
	return errors.Is(err, net.ErrClosed) || errors.Is(err, npipe.ErrClosed)
}
