// +build acceptance
// +build darwin,amd64

package os

import "regexp"

var PackBinaryExp = regexp.MustCompile(`pack-v\d+.\d+.\d+-macos\.`)
