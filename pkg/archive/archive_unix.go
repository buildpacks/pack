//go:build linux || darwin

package archive

import (
	"os"
	"syscall"
)

func hasHardlinks(fi os.FileInfo) bool {
	return fi.Sys().(*syscall.Stat_t).Nlink > 1
}

func getInodeFromStat(stat interface{}) (inode uint64, err error) {
	s, ok := stat.(*syscall.Stat_t)
	if ok {
		inode = s.Ino
	}
	return
}
