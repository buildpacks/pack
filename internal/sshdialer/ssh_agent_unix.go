//go:build aix || darwin || dragonfly || freebsd || linux || netbsd || openbsd || solaris
// +build aix darwin dragonfly freebsd linux netbsd openbsd solaris

package sshdialer

import "net"

func dialSSHAgent(addr string) (net.Conn, error) {
	return net.Dial("unix", addr)
}
