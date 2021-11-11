//go:build acceptance && !windows
// +build acceptance,!windows

package os

import "os"

const PackBinaryName = "pack"

var InterruptSignal = os.Interrupt
