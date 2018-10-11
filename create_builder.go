package pack

import (
	"context"
	"fmt"
	"github.com/BurntSushi/toml"
	"github.com/buildpack/lifecycle"
	"github.com/buildpack/lifecycle/img"
	"github.com/buildpack/pack/config"
	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/network"
	"github.com/google/go-containerregistry/pkg/v1"
	"github.com/pkg/errors"
	"io"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"strings"
)

type BuilderTOML struct {
	Buildpacks []struct{
		ID     string `toml:"id"`
		URI    string `toml:"uri"`
		Latest bool   `toml:"latest"`
	}                `toml:"buildpacks"`
	Groups     []lifecycle.BuildpackGroup `toml:"groups"`
}

type BuilderConfig struct {
	RepoName   string
	Repo       img.Store
	Buildpacks []Buildpack
	Groups     []lifecycle.BuildpackGroup
	BaseImage  v1.Image
	BuilderDir string //original location of builder.toml, used for interpreting relative paths in buildpack URIs
}
type Buildpack struct {
	ID     string
	Dir    string
	Latest bool
}


//go:generate mockgen -package mocks -destination mocks/docker.go github.com/buildpack/pack Docker
type Docker interface {
	PullImage(ref string) error
	RunContainer(ctx context.Context, id string, stdout io.Writer, stderr io.Writer) error
	VolumeRemove(ctx context.Context, volumeID string, force bool) error
	ContainerCreate(ctx context.Context, config *container.Config, hostConfig *container.HostConfig, networkingConfig *network.NetworkingConfig, containerName string) (container.ContainerCreateCreatedBody, error)
	ContainerRemove(ctx context.Context, containerID string, options types.ContainerRemoveOptions) error
	CopyToContainer(ctx context.Context, containerID, dstPath string, content io.Reader, options types.CopyToContainerOptions) error
	CopyFromContainer(ctx context.Context, containerID, srcPath string) (io.ReadCloser, types.ContainerPathStat, error)
	ImageBuild(ctx context.Context, buildContext io.Reader, options types.ImageBuildOptions) (types.ImageBuildResponse, error)
	ImageInspectWithRaw(ctx context.Context, imageID string) (types.ImageInspect, []byte, error)
}

//go:generate mockgen -package mocks -destination mocks/images.go github.com/buildpack/pack Images
type Images interface {
	ReadImage(repoName string, useDaemon bool) (v1.Image, error)
	RepoStore(repoName string, useDaemon bool) (img.Store, error)
}

type BuilderFactory struct {
	Log    *log.Logger
	Docker Docker
	FS     FS
	Config *config.Config
	Images Images
}

//go:generate mockgen -package mocks -destination mocks/fs.go github.com/buildpack/pack FS
type FS interface {
	CreateTGZFile(tarFile, srcDir, tarDir string, uid, gid int) error
	CreateTarReader(srcDir, tarDir string, uid, gid int) (io.Reader, chan error)
	Untar(r io.Reader, dest string) error
	CreateSingleFileTar(path, txt string) (io.Reader, error)
}

type CreateBuilderFlags struct {
	RepoName        string
	BuilderTomlPath string
	StackID         string
	Publish         bool
	NoPull          bool
}

func (f *BuilderFactory) BuilderConfigFromFlags(flags CreateBuilderFlags) (BuilderConfig, error) {
	baseImage, err := f.baseImageName(flags.StackID, flags.RepoName)
	if err != nil {
		return BuilderConfig{}, err
	}
	if !flags.NoPull && !flags.Publish {
		f.Log.Println("Pulling builder base image ", baseImage)
		err := f.Docker.PullImage(baseImage)
		if err != nil {
			return BuilderConfig{}, fmt.Errorf(`failed to pull stack build image "%s": %s`, baseImage, err)
		}
	}

	builderConfig := BuilderConfig{RepoName: flags.RepoName}
	builderConfig.BuilderDir = filepath.Dir(flags.BuilderTomlPath)
	builderConfig.BaseImage, err = f.Images.ReadImage(baseImage, !flags.Publish)
	if err != nil {
		return BuilderConfig{}, fmt.Errorf(`failed to read base image "%s": %s`, baseImage, err)
	}
	if builderConfig.BaseImage == nil {
		return BuilderConfig{}, fmt.Errorf(`base image "%s" was not found`, baseImage)
	}
	builderConfig.Repo, err = f.Images.RepoStore(flags.RepoName, !flags.Publish)
	if err != nil {
		return BuilderConfig{}, fmt.Errorf(`failed to create repository store for builder image "%s": %s`, flags.RepoName, err)
	}

	builderTOML := &BuilderTOML{}
	_, err = toml.DecodeFile(flags.BuilderTomlPath, &builderTOML)
	if err != nil {
		return BuilderConfig{}, fmt.Errorf(`failed to decode builder config from file "%s": %s`, flags.BuilderTomlPath, err)
	}
	builderConfig.Groups = builderTOML.Groups
	for _, b := range builderTOML.Buildpacks {
		dir := strings.TrimPrefix(b.URI, "file://")
		if !filepath.IsAbs(dir) {
			dir = filepath.Join(builderConfig.BuilderDir, dir)
		}
		builderConfig.Buildpacks = append(builderConfig.Buildpacks, Buildpack{
			ID: b.ID,
			Latest: b.Latest,
			Dir: dir,
		})
	}
	return builderConfig, nil
}

func (f *BuilderFactory) baseImageName(stackID, repoName string) (string, error) {
	stack, err := f.Config.Get(stackID)
	if err != nil {
		return "", err
	}
	if len(stack.BuildImages) == 0 {
		return "", fmt.Errorf(`Invalid stack: stack "%s" requires at least one build image`, stack.ID)
	}
	registry, err := config.Registry(repoName)
	if err != nil {
		return "", err
	}
	return config.ImageByRegistry(registry, stack.BuildImages)
}

func (f *BuilderFactory) Create(config BuilderConfig) error {
	tmpDir, err := ioutil.TempDir("", "create-builder") // TODO
	if err != nil {
		return fmt.Errorf(`failed to create temporary directory: %s`, err)
	}
	defer os.Remove(tmpDir) // TODO

	orderTar, err := f.orderLayer(tmpDir, config.Groups)
	if err != nil {
		return fmt.Errorf(`failed generate order.toml layer: %s`, err)
	}
	builderImage, _, err := img.Append(config.BaseImage, orderTar)
	if err != nil {
		return fmt.Errorf(`failed append order.toml layer to image: %s`, err)
	}
	for _, buildpack := range config.Buildpacks {
		tarFile, err := f.buildpackLayer(tmpDir, buildpack, config.BuilderDir)
		if err != nil {
			return fmt.Errorf(`failed generate layer for buildpack "%s": %s`, buildpack.ID, err)
		}
		builderImage, _, err = img.Append(builderImage, tarFile)
		if err != nil {
			return fmt.Errorf(`failed append buildpack layer to image: %s`, err)
		}
	}
	tarFile, err := f.latestLayer(config.Buildpacks, tmpDir, config.BuilderDir)
	if err != nil {
		return fmt.Errorf(`failed generate layer for latest links: %s`, err)
	}
	builderImage, _, err = img.Append(builderImage, tarFile)
	if err != nil {
		return fmt.Errorf(`failed append latest link layer to image: %s`, err)
	}

	if err := config.Repo.Write(builderImage); err != nil {
		return err
	}

	f.Log.Println("Successfully created builder image:", config.RepoName)
	f.Log.Println("")
	f.Log.Println(`Tip: Run "pack build <image name> --builder <builder image> --path <app source code>" to use this builder`)

	return nil
}

type order struct {
	Groups []lifecycle.BuildpackGroup `toml:"groups"`
}

func (f *BuilderFactory) orderLayer(dest string, groups []lifecycle.BuildpackGroup) (layerTar string, err error) {
	buildpackDir := filepath.Join(dest, "buildpack")
	err = os.Mkdir(buildpackDir, 0755)
	if err != nil {
		return "", err
	}

	orderFile, err := os.Create(filepath.Join(buildpackDir, "order.toml"))
	if err != nil {
		return "", err
	}
	defer orderFile.Close()
	err = toml.NewEncoder(orderFile).Encode(order{Groups: groups})
	if err != nil {
		return "", err
	}
	layerTar = filepath.Join(dest, "order.tar")
	if err := f.FS.CreateTGZFile(layerTar, buildpackDir, "/buildpacks", 0, 0); err != nil {
		return "", err
	}
	return layerTar, nil
}

type BuildpackData struct {
	BP struct {
		ID      string `toml:"id"`
		Version string `toml:"version"`
	} `toml:"buildpack"`
}

func (f *BuilderFactory) buildpackLayer(dest string, buildpack Buildpack, builderDir string) (layerTar string, err error) {
	data, err := f.buildpackData(buildpack, buildpack.Dir)
	if err != nil {
		return "", err
	}
	bp := data.BP
	if buildpack.ID != bp.ID {
		return "", fmt.Errorf("buildpack ids did not match: %s != %s", buildpack.ID, bp.ID)
	}
	if bp.Version == "" {
		return "", fmt.Errorf("buildpack.toml must provide version: %s", filepath.Join(buildpack.Dir, "buildpack.toml"))
	}
	tarFile := filepath.Join(dest, fmt.Sprintf("%s.%s.tar", buildpack.ID, bp.Version))
	if err := f.FS.CreateTGZFile(tarFile, buildpack.Dir, filepath.Join("/buildpacks", buildpack.ID, bp.Version), 0, 0); err != nil {
		return "", err
	}
	return tarFile, err
}

func (f *BuilderFactory) buildpackData(buildpack Buildpack, dir string) (*BuildpackData, error) {
	data := &BuildpackData{}
	_, err := toml.DecodeFile(filepath.Join(dir, "buildpack.toml"), &data)
	if err != nil {
		return nil, errors.Wrapf(err, "reading buildpack.toml from buildpack: %s", dir)
	}
	return data, nil
}

func (f *BuilderFactory) latestLayer(buildpacks []Buildpack, dest, builderDir string) (string, error) {
	tmpDir, err := ioutil.TempDir(dest, "create-builder-latest")
	if err != nil {
		return "", err
	}
	for _, bp := range buildpacks {
		if bp.Latest {
			data, err := f.buildpackData(bp, bp.Dir)
			if err != nil {
				return "", err
			}
			err = os.Mkdir(filepath.Join(tmpDir, bp.ID), 0755)
			if err != nil {
				return "", err
			}
			err = os.Symlink(filepath.Join("/", "buildpacks", bp.ID, data.BP.Version), filepath.Join(tmpDir, bp.ID, "latest"))
			if err != nil {
				fmt.Println("E")
			}
		}
	}
	tarFile := filepath.Join(dest, fmt.Sprintf("%s.%s.tar", "latest", "buildpacks"))
	if err := f.FS.CreateTGZFile(tarFile, tmpDir, filepath.Join("/buildpacks"), 0, 0); err != nil {
		return "", err
	}
	return tarFile, err
}
