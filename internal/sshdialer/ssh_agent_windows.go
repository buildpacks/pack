package sshdialer

import (
	"net"
	"strings"

	"gopkg.in/natefinch/npipe.v2"
)

func dialSSHAgent(addr string) (net.Conn, error) {
	if strings.Contains(addr, "\\pipe\\") {
		return npipe.Dial(addr)
	} else {
		return net.Dial("unix", addr)
	}
}
