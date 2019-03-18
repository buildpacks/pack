package build

import (
	"archive/tar"
	"context"
	"fmt"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/volume"
	"io"
	"io/ioutil"
	"math/rand"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/BurntSushi/toml"
	"github.com/buildpack/lifecycle"
	"github.com/buildpack/lifecycle/image"

	"github.com/buildpack/pack/docker"
	"github.com/buildpack/pack/fs"
	"github.com/buildpack/pack/style"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/network"
	"github.com/pkg/errors"

	"github.com/buildpack/pack/logging"
)

type Lifecycle struct {
	BuilderImage    string
	Logger          *logging.Logger
	Docker          Docker
	WorkspaceVolume string
	uid, gid        int
	appDir          string
	appOnce         *sync.Once
}

type Docker interface {
	RunContainer(ctx context.Context, id string, stdout io.Writer, stderr io.Writer) error
	CopyToContainer(ctx context.Context, containerID, dstPath string, content io.Reader, options types.CopyToContainerOptions) error
	ContainerCreate(ctx context.Context, config *container.Config, hostConfig *container.HostConfig, networkingConfig *network.NetworkingConfig, containerName string) (container.ContainerCreateCreatedBody, error)
	ContainerRemove(ctx context.Context, containerID string, options types.ContainerRemoveOptions) error
	ImageRemove(ctx context.Context, imageID string, options types.ImageRemoveOptions) ([]types.ImageDeleteResponseItem, error)
	ImageList(ctx context.Context, options types.ImageListOptions) ([]types.ImageSummary, error)
	VolumeRemove(ctx context.Context, volumeID string, force bool) error
	VolumeList(ctx context.Context, filter filters.Args) (volume.VolumeListOKBody, error)
}

const (
	launchDir = "/workspace"
)

type LifecycleConfig struct {
	BuilderImage string
	Logger       *logging.Logger
	EnvFile      map[string]string
	Buildpacks   []string
	AppDir       string
}

func init() {
	rand.Seed(time.Now().UTC().UnixNano())
}

func NewLifecycle(c LifecycleConfig) (*Lifecycle, error) {
	client, err := docker.New()
	if err != nil {
		return nil, err
	}
	factory, err := image.NewFactory()
	if err != nil {
		return nil, err
	}

	builder, err := factory.NewLocal(c.BuilderImage)
	if err != nil {
		return nil, err
	}
	builder.Rename(fmt.Sprintf("pack.local/builder/%x", randString(10)))
	uid, gid, err := packUidGid(builder)
	if err != nil {
		return nil, err
	}

	tmpDir, err := ioutil.TempDir("", "pack.build.tars")
	defer os.RemoveAll(tmpDir)
	if err != nil {
		return nil, err
	}

	envTar, err := tarEnvFile(tmpDir, c.EnvFile)
	defer os.RemoveAll(envTar)
	if err != nil {
		return nil, err
	}
	if err := builder.AddLayer(envTar); err != nil {
		return nil, err
	}

	if len(c.Buildpacks) != 0 {
		tars, err := createBuildpacksTars(tmpDir, c.Buildpacks, c.Logger, uid, gid)
		if err != nil {
			return nil, err
		}

		for _, t := range tars {
			if err := builder.AddLayer(t); err != nil {
				return nil, err
			}
		}
	}

	if _, err := builder.Save(); err != nil {
		return nil, err
	}

	return &Lifecycle{
		BuilderImage:    builder.Name(),
		Logger:          c.Logger,
		Docker:          client,
		WorkspaceVolume: "pack-workspace-" + randString(10),
		appDir:          c.AppDir,
		uid:             uid,
		gid:             gid,
		appOnce:         &sync.Once{},
	}, nil
}

func (l *Lifecycle) Cleanup() error {
	_, err := l.Docker.ImageRemove(context.Background(), l.BuilderImage, types.ImageRemoveOptions{})
	if err != nil {
		return err
	}
	return l.Docker.VolumeRemove(context.Background(), l.WorkspaceVolume, true)
}

func randString(n int) string {
	b := make([]byte, n)
	for i := range b {
		b[i] = 'a' + byte(rand.Intn(26))
	}
	return string(b)
}

func packUidGid(builder image.Image) (int, int, error) {
	sUID, err := builder.Env("PACK_USER_ID")
	if err != nil {
		return 0, 0, errors.Wrap(err, "reading builder env variables")
	}
	sGID, err := builder.Env("PACK_GROUP_ID")
	if err != nil {
		return 0, 0, errors.Wrap(err, "reading builder env variables")
	}
	var uid, gid int
	uid, err = strconv.Atoi(sUID)
	if err != nil {
		return 0, 0, errors.Wrapf(err, "parsing pack uid: %s", sUID)
	}
	gid, err = strconv.Atoi(sGID)
	if err != nil {
		return 0, 0, errors.Wrapf(err, "parsing pack gid: %s", sGID)
	}
	return uid, gid, nil
}

func tarEnvFile(tmpDir string, env map[string]string) (string, error) {
	now := time.Now()
	fh, err := os.Create(filepath.Join(tmpDir, "env.tar"))
	defer fh.Close()
	if err != nil {
		return "", err
	}
	tw := tar.NewWriter(fh)
	defer tw.Close()
	for k, v := range env {
		if err := tw.WriteHeader(&tar.Header{Name: "/platform/env/" + k, Size: int64(len(v)), Mode: 0444, ModTime: now}); err != nil {
			return "", err
		}
		if _, err := tw.Write([]byte(v)); err != nil {
			return "", err
		}
	}
	if err := tw.WriteHeader(&tar.Header{Typeflag: tar.TypeDir, Name: "/platform/env/", Mode: 0555, ModTime: now}); err != nil {
		return "", err
	}
	return fh.Name(), nil
}

func createBuildpacksTars(tmpDir string, buildpacks []string, logger *logging.Logger, uid int, gid int) ([]string, error) {
	tars := make([]string, 0, len(buildpacks)+1)

	var buildpackGroup []*lifecycle.Buildpack
	fs := fs.FS{}
	for _, bp := range buildpacks {
		var id, version string
		if _, err := os.Stat(filepath.Join(bp, "buildpack.toml")); !os.IsNotExist(err) {
			if runtime.GOOS == "windows" {
				return nil, fmt.Errorf("directory buildpacks are not implemented on windows")
			}
			var buildpackTOML struct {
				Buildpack lifecycle.Buildpack
			}

			_, err = toml.DecodeFile(filepath.Join(bp, "buildpack.toml"), &buildpackTOML)
			if err != nil {
				return nil, fmt.Errorf(`failed to decode buildpack.toml from "%s": %s`, bp, err)
			}
			id = buildpackTOML.Buildpack.ID
			version = buildpackTOML.Buildpack.Version

			tarFile := filepath.Join(tmpDir, fmt.Sprintf("%s.%s.tar", id, version))

			if err := fs.CreateTarFile(tarFile, bp, filepath.Join("/buildpacks", id, version), uid, gid); err != nil {
				return nil, err
			}

			tars = append(tars, tarFile)
		} else {
			id, version = parseBuildpack(bp, logger)
		}
		buildpackGroup = append(
			buildpackGroup,
			&lifecycle.Buildpack{ID: id, Version: version, Optional: false},
		)
	}

	orderTarPath, err := orderTar(tmpDir, buildpackGroup)
	if err != nil {
		return nil, err
	}
	tars = append(tars, orderTarPath)
	return tars, nil
}

func orderTar(tmpDir string, buildpacks []*lifecycle.Buildpack) (string, error) {
	groups := lifecycle.BuildpackOrder{
		lifecycle.BuildpackGroup{
			Buildpacks: buildpacks,
		},
	}

	var tomlBuilder strings.Builder
	if err := toml.NewEncoder(&tomlBuilder).Encode(map[string]interface{}{"groups": groups}); err != nil {
		return "", errors.Wrapf(err, "encoding order.toml: %#v", groups)
	}

	orderToml := tomlBuilder.String()
	err := (&fs.FS{}).CreateSingleFileTar(
		filepath.Join(tmpDir, "order.tar"),
		"/buildpacks/order.toml",
		orderToml,
	)
	if err != nil {
		return "", errors.Wrap(err, "converting order TOML to tar reader")
	}
	return filepath.Join(tmpDir, "order.tar"), nil
}

func parseBuildpack(ref string, logger *logging.Logger) (string, string) {
	parts := strings.Split(ref, "@")
	if len(parts) == 2 {
		return parts[0], parts[1]
	}
	logger.Verbose("No version for %s buildpack provided, will use %s", style.Symbol(parts[0]), style.Symbol(parts[0]+"@latest"))
	return parts[0], "latest"
}
