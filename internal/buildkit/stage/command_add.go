package stage

// import (
// 	"path"
// 	"path/filepath"
// 	"strings"

// 	"github.com/buildpacks/pack/internal/buildkit/packerfile/options"
// 	digest "github.com/opencontainers/go-digest"
// )

// // ADDCommand implements packerfile.Packerfile.
// func (s *Stage) ADDCommand(add string, ops options.ADD) (err error) {
// 	var checksum digest.Digest
// 	if s.digest != "" {
// 		checksum, err = digest.Parse(s.digest)
// 	}

// 	if err == nil {
// 		err = s.COPYCommand("", []string{}, options.COPY{
// 			Chown: ops.Chown,
// 			Chmod: ops.Chmod,
// 			Exclude: ops.Exclude,
// 			Link: ops.Link,
// 		})
// 		err = dispatchCopy(d, copyConfig{
// 			params:          c.SourcesAndDest,
// 			excludePatterns: c.ExcludePatterns,
// 			source:          opt.buildContext,
// 			isAddCommand:    true,
// 			cmdToPrint:      c,
// 			chown:           c.Chown,
// 			chmod:           c.Chmod,
// 			link:            c.Link,
// 			keepGitDir:      c.KeepGitDir,
// 			checksum:        checksum,
// 			location:        c.Location(),
// 			opt:             opt,
// 		})
// 	}

// 	if err == nil {
// 		for _, src := range c.SourcePaths {
// 			if !strings.HasPrefix(src, "http://") && !strings.HasPrefix(src, "https://") {
// 				d.ctxPaths[path.Join("/", filepath.ToSlash(src))] = struct{}{}
// 			}
// 		}
// 	}

// 	return err
// }
