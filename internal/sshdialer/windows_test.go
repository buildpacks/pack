//go:build windows
// +build windows

package sshdialer_test

import (
	"errors"
	"fmt"
	"net"
	"os"
	"strings"

	"github.com/hectane/go-acl"
	"gopkg.in/natefinch/npipe.v2"
)

func fixupPrivateKeyMod(path string) {
	err := acl.Chmod(path, 0400)
	fmt.Fprintf(os.Stderr, "fixup err: %v", err)
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
