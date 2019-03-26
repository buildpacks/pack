package pack

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
	"github.com/buildpack/lifecycle"
	lcimg "github.com/buildpack/lifecycle/image"
	"github.com/pkg/errors"

	"github.com/buildpack/pack/archive"
	"github.com/buildpack/pack/builder"
	"github.com/buildpack/pack/buildpack"
	"github.com/buildpack/pack/config"
	"github.com/buildpack/pack/logging"
	"github.com/buildpack/pack/stack"
	"github.com/buildpack/pack/style"
)

type BuilderConfig struct {
	Buildpacks      []buildpack.Buildpack
	Groups          []lifecycle.BuildpackGroup
	Repo            lcimg.Image
	BuilderDir      string // original location of builder.toml, used for interpreting relative paths in buildpack URIs
	RunImage        string
	RunImageMirrors []string
}

type BuilderFactory struct {
	Logger           *logging.Logger
	Config           *config.Config
	Fetcher          Fetcher
	BuildpackFetcher BuildpackFetcher
}

type CreateBuilderFlags struct {
	RepoName        string
	BuilderTomlPath string
	Publish         bool
	NoPull          bool
}

func (f *BuilderFactory) BuilderConfigFromFlags(ctx context.Context, flags CreateBuilderFlags) (BuilderConfig, error) {
	builderConfig := BuilderConfig{}
	builderConfig.BuilderDir = filepath.Dir(flags.BuilderTomlPath)

	builderTOML := &builder.TOML{}
	_, err := toml.DecodeFile(flags.BuilderTomlPath, &builderTOML)
	if err != nil {
		return BuilderConfig{}, fmt.Errorf(`failed to decode builder config from file %s: %s`, flags.BuilderTomlPath, err)
	}

	if err := validateBuilderTOML(builderTOML); err != nil {
		return BuilderConfig{}, err
	}

	baseImage := builderTOML.Stack.BuildImage
	builderConfig.RunImage = builderTOML.Stack.RunImage
	builderConfig.RunImageMirrors = builderTOML.Stack.RunImageMirrors
	if flags.Publish {
		builderConfig.Repo, err = f.Fetcher.FetchRemoteImage(baseImage)
	} else {
		if !flags.NoPull {
			builderConfig.Repo, err = f.Fetcher.FetchUpdatedLocalImage(ctx, baseImage, f.Logger.RawVerboseWriter())
		} else {
			builderConfig.Repo, err = f.Fetcher.FetchLocalImage(baseImage)
		}
	}
	if err != nil {
		return BuilderConfig{}, errors.Wrapf(err, "opening base image: %s", baseImage)
	}
	builderConfig.Repo.Rename(flags.RepoName)

	builderConfig.Groups = builderTOML.Groups

	for _, b := range builderTOML.Buildpacks {
		fetchedBuildpack, err := f.BuildpackFetcher.FetchBuildpack(builderConfig.BuilderDir, b)
		if err != nil {
			return BuilderConfig{}, err
		}
		builderConfig.Buildpacks = append(builderConfig.Buildpacks, fetchedBuildpack)
	}
	return builderConfig, nil
}

func (f *BuilderFactory) Create(config BuilderConfig) error {
	tmpDir, err := ioutil.TempDir("", "create-builder")
	if err != nil {
		return fmt.Errorf(`failed to create temporary directory: %s`, err)
	}
	defer os.RemoveAll(tmpDir)

	orderTar, err := f.orderLayer(tmpDir, config.Groups)
	if err != nil {
		return fmt.Errorf(`failed to generate order.toml layer: %s`, err)
	}
	if err := config.Repo.AddLayer(orderTar); err != nil {
		return fmt.Errorf(`failed append order.toml layer to image: %s`, err)
	}

	buildpacksMetadata := make([]builder.BuildpackMetadata, 0, len(config.Buildpacks))
	for _, buildpack := range config.Buildpacks {
		tarFile, err := f.buildpackLayer(tmpDir, &buildpack, config.BuilderDir)
		if err != nil {
			return fmt.Errorf(`failed to generate layer for buildpack %s: %s`, style.Symbol(buildpack.ID), err)
		}
		if err := config.Repo.AddLayer(tarFile); err != nil {
			return fmt.Errorf(`failed append buildpack layer to image: %s`, err)
		}
		buildpacksMetadata = append(buildpacksMetadata, builder.BuildpackMetadata{ID: buildpack.ID, Version: buildpack.Version, Latest: buildpack.Latest})
	}

	tarFile, err := f.latestLayer(config.Buildpacks, tmpDir, config.BuilderDir)
	if err != nil {
		return fmt.Errorf(`failed generate layer for latest links: %s`, err)
	}
	if err := config.Repo.AddLayer(tarFile); err != nil {
		return fmt.Errorf(`failed append latest link layer to image: %s`, err)
	}

	groupsMetadata := make([]builder.GroupMetadata, 0, len(config.Groups))
	for _, group := range config.Groups {
		groupBuildpacks := make([]builder.BuildpackMetadata, 0, len(group.Buildpacks))
		for _, buildpack := range group.Buildpacks {
			groupBuildpacks = append(groupBuildpacks, builder.BuildpackMetadata{ID: buildpack.ID, Version: buildpack.Version})
		}
		groupsMetadata = append(groupsMetadata, builder.GroupMetadata{Buildpacks: groupBuildpacks})
	}

	jsonBytes, err := json.Marshal(&builder.Metadata{
		Stack: stack.Metadata{
			RunImage: stack.RunImageMetadata{
				Image:   config.RunImage,
				Mirrors: config.RunImageMirrors,
			},
		},
		Buildpacks: buildpacksMetadata,
		Groups:     groupsMetadata,
	})
	if err != nil {
		return fmt.Errorf(`failed marshal builder image metadata: %s`, err)
	}

	if err := config.Repo.SetLabel(builder.MetadataLabel, string(jsonBytes)); err != nil {
		return fmt.Errorf("failed to set metadata label: %s", err)
	}

	stackTar, err := f.stackLayer(tmpDir, config.RunImage, config.RunImageMirrors)
	if err != nil {
		return fmt.Errorf(`failed to generate stack.toml layer: %s`, err)
	}
	if err := config.Repo.AddLayer(stackTar); err != nil {
		return fmt.Errorf(`failed to append stack.toml layer to image: %s`, err)
	}

	if err := config.Repo.SetEnv("CNB_STACK_PATH", filepath.Join("/buildpacks", "stack.toml")); err != nil {
		return err
	}

	if _, err := config.Repo.Save(); err != nil {
		return err
	}

	return nil
}

type order struct {
	Groups []lifecycle.BuildpackGroup `toml:"groups"`
}

func (f *BuilderFactory) orderLayer(dest string, groups []lifecycle.BuildpackGroup) (layerTar string, err error) {
	bpDir := filepath.Join(dest, "buildpacks")
	err = os.Mkdir(bpDir, 0755)
	if err != nil {
		return "", err
	}

	orderFile, err := os.OpenFile(filepath.Join(bpDir, "order.toml"), os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0644)
	if err != nil {
		return "", err
	}
	defer orderFile.Close()
	err = toml.NewEncoder(orderFile).Encode(order{Groups: groups})
	if err != nil {
		return "", err
	}
	layerTar = filepath.Join(dest, "order.tar")
	if err := archive.CreateTar(layerTar, bpDir, "/buildpacks", 0, 0); err != nil {
		return "", err
	}
	return layerTar, nil
}

func (f *BuilderFactory) stackLayer(dest string, runImage string, mirrors []string) (layerTar string, err error) {
	bpDir := filepath.Join(dest, "buildpacks")
	if err := os.MkdirAll(bpDir, 0755); err != nil {
		return "", err
	}

	stackFile, err := os.OpenFile(filepath.Join(bpDir, "stack.toml"), os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0644)
	if err != nil {
		return "", err
	}
	defer stackFile.Close()

	content := stack.Metadata{
		RunImage: stack.RunImageMetadata{
			Image:   runImage,
			Mirrors: mirrors,
		},
	}
	if err = toml.NewEncoder(stackFile).Encode(&content); err != nil {
		return "", err
	}

	layerTar = filepath.Join(dest, "stack.tar")
	if err := archive.CreateTar(layerTar, bpDir, "/buildpacks", 0, 0); err != nil {
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

// buildpackLayer creates and returns the location of a tgz file for a buildpack layer. That file will reside in the `dest` directory.
// The tgz file is either created from an initially local directory, or it is downloaded (and validated) from
// a remote location if the buildpack uri uses the http(s) protocol.
func (f *BuilderFactory) buildpackLayer(dest string, buildpack *buildpack.Buildpack, builderDir string) (layerTar string, err error) {
	dir := buildpack.Dir

	data, err := f.buildpackData(*buildpack, dir)
	if err != nil {
		return "", err
	}
	bp := data.BP
	if buildpack.ID != bp.ID {
		return "", fmt.Errorf("buildpack IDs did not match: %s != %s", buildpack.ID, bp.ID)
	}
	if bp.Version == "" {
		return "", fmt.Errorf("buildpack.toml must provide version: %s", filepath.Join(buildpack.Dir, "buildpack.toml"))
	}

	buildpack.Version = bp.Version
	tarFile := filepath.Join(dest, fmt.Sprintf("%s.%s.tar", buildpack.EscapedID(), bp.Version))
	if err := archive.CreateTar(tarFile, dir, filepath.Join("/buildpacks", buildpack.EscapedID(), bp.Version), 0, 0); err != nil {
		return "", err
	}
	return tarFile, err
}

func (f *BuilderFactory) buildpackData(buildpack buildpack.Buildpack, dir string) (*BuildpackData, error) {
	data := &BuildpackData{}
	_, err := toml.DecodeFile(filepath.Join(dir, "buildpack.toml"), &data)
	if err != nil {
		return nil, errors.Wrapf(err, "reading buildpack.toml from buildpack: %s", dir)
	}
	return data, nil
}

func (f *BuilderFactory) latestLayer(buildpacks []buildpack.Buildpack, dest, builderDir string) (string, error) {
	layerDir := filepath.Join(dest, "latest-layer")
	err := os.Mkdir(layerDir, 0755)
	if err != nil {
		return "", err
	}
	for _, bp := range buildpacks {
		if bp.Latest {
			data, err := f.buildpackData(bp, bp.Dir)
			if err != nil {
				return "", err
			}
			err = os.Mkdir(filepath.Join(layerDir, bp.EscapedID()), 0755)
			if err != nil {
				return "", err
			}
			err = os.Symlink(filepath.Join("/", "buildpacks", bp.EscapedID(), data.BP.Version), filepath.Join(layerDir, bp.EscapedID(), "latest"))
			if err != nil {
				return "", err
			}
		}
	}
	tarFile := filepath.Join(dest, fmt.Sprintf("%s.%s.tar", "latest", "buildpacks"))
	if err := archive.CreateTar(tarFile, layerDir, "/buildpacks", 0, 0); err != nil {
		return "", err
	}
	return tarFile, nil
}

func validateBuilderTOML(builderTOML *builder.TOML) error {
	if builderTOML == nil {
		return errors.New("builder toml is empty")
	}

	if builderTOML.Stack.ID == "" {
		return errors.New("stack.id is required")
	}

	if builderTOML.Stack.BuildImage == "" {
		return errors.New("stack.build-image is required")
	}

	if builderTOML.Stack.RunImage == "" {
		return errors.New("stack.run-image is required")
	}
	return nil
}
