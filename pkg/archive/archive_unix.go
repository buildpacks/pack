//go:build linux || darwin

package archive

import (
	"os"
	"syscall"
)

func hasHardlinks(fi os.FileInfo, path string) (bool, error) {
	return fi.Sys().(*syscall.Stat_t).Nlink > 1, nil
}

func getInodeFromStat(stat interface{}, path string) (inode uint64, err error) {
	s, ok := stat.(*syscall.Stat_t)
	if ok {
		inode = s.Ino
	}
	return
}
