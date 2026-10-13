package paths

import (
	"net/url"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
)

var schemeRegexp = regexp.MustCompile(`^.+:/.*`)

func IsURI(ref string) bool {
	return schemeRegexp.MatchString(ref)
}

func IsDir(p string) (bool, error) {
	fileInfo, err := os.Stat(p)
	if err != nil {
		return false, err
	}

	return fileInfo.IsDir(), nil
}

// FilePathToURI converts a filepath to URI. If relativeTo is provided not empty and path is
// a relative path it will be made absolute based on the provided value. Otherwise, the
// current working directory is used.
func FilePathToURI(path, relativeTo string) (string, error) {
	if IsURI(path) {
		return path, nil
	}

	if !filepath.IsAbs(path) {
		var err error
		path, err = filepath.Abs(filepath.Join(relativeTo, path))
		if err != nil {
			return "", err
		}
	}

	if runtime.GOOS == "windows" {
		if strings.HasPrefix(path, `\\`) {
			return "file://" + filepath.ToSlash(strings.TrimPrefix(path, `\\`)), nil
		}
		return "file:///" + filepath.ToSlash(path), nil
	}
	return "file://" + path, nil
}

// examples:
//
// - unix file: file://laptop/some%20dir/file.tgz
//
// - windows drive: file:///C:/Documents%20and%20Settings/file.tgz
//
// - windows share: file://laptop/My%20Documents/file.tgz
func URIToFilePath(uri string) (string, error) {
	var (
		osPath string
		err    error
	)

	osPath = filepath.FromSlash(strings.TrimPrefix(uri, "file://"))

	if osPath, err = url.PathUnescape(osPath); err != nil {
		return "", nil
	}

	if runtime.GOOS == "windows" {
		if strings.HasPrefix(osPath, `\`) {
			return strings.TrimPrefix(osPath, `\`), nil
		}
		return `\\` + osPath, nil
	}
	return osPath, nil
}

// reservedNames are the device names reserved on Windows. A name is reserved
// when the part before its first "." equals one of these, case-insensitively,
// so "nul", "CON", and "nul.txt" are all reserved.
// https://docs.microsoft.com/en-us/windows/win32/fileio/naming-a-file?redirectedfrom=MSDN#win32-file-namespaces
var reservedNames = map[string]struct{}{
	"con": {}, "prn": {}, "aux": {}, "nul": {},
	"com1": {}, "com2": {}, "com3": {}, "com4": {}, "com5": {},
	"com6": {}, "com7": {}, "com8": {}, "com9": {},
	"lpt1": {}, "lpt2": {}, "lpt3": {}, "lpt4": {}, "lpt5": {},
	"lpt6": {}, "lpt7": {}, "lpt8": {}, "lpt9": {},
}

// FilterReservedNames rewrites a name that collides with a Windows reserved
// device name into a safe equivalent. These names are only reserved on Windows,
// so on other platforms p is returned unchanged. Only the whole base name is
// matched (case-insensitively), so ordinary names that merely contain a
// reserved word as a substring, such as "config" or "my-company-cache", are
// left untouched.
func FilterReservedNames(p string) string {
	if runtime.GOOS != "windows" {
		return p
	}

	base := p
	ext := ""
	if i := strings.Index(p, "."); i >= 0 {
		base, ext = p[:i], p[i:]
	}

	if _, ok := reservedNames[strings.ToLower(base)]; !ok {
		return p
	}

	// Insert underscores between the characters of the base name so it is no
	// longer a reserved device name, preserving the original casing and any
	// extension (e.g. "nul.txt" -> "n_u_l.txt").
	var sb strings.Builder
	for i, r := range base {
		if i > 0 {
			sb.WriteByte('_')
		}
		sb.WriteRune(r)
	}
	return sb.String() + ext
}

// WindowsDir is equivalent to path.Dir or filepath.Dir but always for Windows paths
// reproduced because Windows implementation is not exported
func WindowsDir(p string) string {
	pathElements := strings.Split(p, `\`)

	dirName := strings.Join(pathElements[:len(pathElements)-1], `\`)

	return dirName
}

// WindowsBasename is equivalent to path.Basename or filepath.Basename but always for Windows paths
// reproduced because Windows implementation is not exported
func WindowsBasename(p string) string {
	pathElements := strings.Split(p, `\`)

	return pathElements[len(pathElements)-1]
}

// WindowsToSlash is equivalent to path.ToSlash or filepath.ToSlash but always for Windows paths
// reproduced because Windows implementation is not exported
func WindowsToSlash(p string) string {
	slashPath := strings.ReplaceAll(p, `\`, "/") // convert slashes
	if len(slashPath) < 2 {
		return ""
	}

	return slashPath[2:] // strip volume
}

// WindowsPathSID returns the appropriate SID for a given UID and GID
// This is the basic logic for path permissions in Pack and Lifecycle
func WindowsPathSID(uid, gid int) string {
	if uid == 0 && gid == 0 {
		return "S-1-5-32-544" // BUILTIN\Administrators
	}
	return "S-1-5-32-545" // BUILTIN\Users
}

// CanonicalTarPath return a cleaned path (see path.Clean) with leading slashes removed
func CanonicalTarPath(p string) string {
	return strings.TrimPrefix(path.Clean(p), "/")
}
