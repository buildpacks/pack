// +build acceptance
// +build !windows

package variables

import "os"

const PackBinaryName = "pack"

var InterruptSignal = os.Interrupt
