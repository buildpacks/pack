package build

import (
	"archive/tar"
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
	"github.com/buildpacks/lifecycle/buildpack"
	"github.com/buildpacks/lifecycle/cmd"

	"github.com/buildpacks/pack/pkg/archive"
	"github.com/buildpacks/pack/pkg/logging"
)

const (
	DockerfileKindBuild = "build"
	DockerfileKindRun   = "run"
)

type Extensions struct {
	Extensions []buildpack.GroupElement
}

type DockerfileInfo struct {
	Info *buildpack.DockerfileInfo
	Args []Arg
}

type Arg struct {
	Name  string `toml:"name"`
	Value string `toml:"value"`
}

type Config struct {
	Build BuildConfig `toml:"build"`
	Run   BuildConfig `toml:"run"`
}

type BuildConfig struct {
	Args []Arg `toml:"args"`
}

func (extensions *Extensions) DockerFiles(kind string, path string, logger logging.Logger) ([]DockerfileInfo, error) {
	var dockerfiles []DockerfileInfo
	for _, ext := range extensions.Extensions {
		dockerfile, err := extensions.ReadDockerFile(path, kind, ext.ID)
		if err != nil {
			return nil, err
		}
		if dockerfile != nil {
			logger.Debugf("Found %s Dockerfile for extension '%s'", kind, ext.ID)
			switch kind {
			case DockerfileKindBuild:
				break
			case DockerfileKindRun:
				buildpack.ValidateRunDockerfile(dockerfile.Info, logger)
			default:
				return nil, fmt.Errorf("unknown dockerfile kind: %s", kind)
			}
			dockerfiles = append(dockerfiles, *dockerfile)
		}
	}
	return dockerfiles, nil
}

func (extensions *Extensions) ReadDockerFile(path string, kind string, extID string) (*DockerfileInfo, error) {
	dockerfilePath := filepath.Join(path, kind, escapeID(extID), "Dockerfile")
	if _, err := os.Stat(dockerfilePath); err != nil {
		return nil, nil
	}
	configPath := filepath.Join(path, kind, escapeID(extID), "extend-config.toml")
	var config Config
	_, err := toml.DecodeFile(configPath, &config)
	if err != nil {
		if !os.IsNotExist(err) {
			return nil, err
		}
	}

	var args []Arg
	if kind == buildpack.DockerfileKindBuild {
		args = config.Build.Args
	} else {
		args = config.Run.Args
	}
	return &DockerfileInfo{
		Info: &buildpack.DockerfileInfo{
			ExtensionID: extID,
			Kind:        kind,
			Path:        dockerfilePath,
		},
		Args: args,
	}, nil
}

func (extensions *Extensions) SetExtensions(path string, logger logging.Logger) error {
	groupExt, err := readExtensionsGroup(path)
	if err != nil {
		return fmt.Errorf("reading group: %w", err)
	}
	for i := range groupExt {
		groupExt[i].Extension = true
	}
	for _, groupEl := range groupExt {
		if err = cmd.VerifyBuildpackAPI(groupEl.Kind(), groupEl.String(), groupEl.API, logger); err != nil {
			return err
		}
	}
	extensions.Extensions = groupExt
	return nil
}

func readExtensionsGroup(path string) ([]buildpack.GroupElement, error) {
	var group buildpack.Group
	_, err := toml.DecodeFile(filepath.Join(path, "group.toml"), &group)
	for e := range group.GroupExtensions {
		group.GroupExtensions[e].Extension = true
		group.GroupExtensions[e].Optional = true
	}
	return group.GroupExtensions, err
}

func escapeID(id string) string {
	return strings.ReplaceAll(id, "/", "_")
}

func (dockerfile *DockerfileInfo) CreateBuildContext(path string) (io.Reader, error) {
	defaultFilterFunc := func(file string) bool { return true }
	buf := new(bytes.Buffer)
	tarWriter := tar.NewWriter(buf)
	var completeErr error

	defer func() {
		if err := tarWriter.Close(); err != nil {
			fmt.Println("Error closing tar writer:", err)
			completeErr = archive.AggregateError(completeErr, err)
		}
	}()
	if err := archive.WriteDirToTar(tarWriter, path, "/workspace", 0, 0, -1, true, false, defaultFilterFunc); err != nil {
		tarWriter.Close()
		fmt.Println("Error adding workspace:", err)
		completeErr = archive.AggregateError(completeErr, err)
	}

	if err := archive.WriteFileToTar(tarWriter, dockerfile.Info.Path, filepath.Join(".", "Dockerfile"), 0, 0, -1, true); err != nil {
		tarWriter.Close()
		fmt.Println("Error adding dockerfile:", err)
		completeErr = archive.AggregateError(completeErr, err)
	}

	return buf, completeErr
}
