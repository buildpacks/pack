package archive

import (
	"os"
)

func hasHardlinks(fi os.FileInfo) bool {
	return false
}

func getInodeFromStat(stat interface{}) (inode uint64, err error) {
	return
}
