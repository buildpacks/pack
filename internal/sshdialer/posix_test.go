//+build !windows

package sshdialer_test

import "os"

func fixupPrivateKeyMod(path string) {
	err := os.Chmod(path, 0400)
	if err != nil {
		panic(err)
	}
}
