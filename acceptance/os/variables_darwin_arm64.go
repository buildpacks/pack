// +build acceptance
// +build darwin,arm64

package os

import "regexp"

var PackBinaryExp = regexp.MustCompile(`pack-v\d+.\d+.\d+-macos-`)
