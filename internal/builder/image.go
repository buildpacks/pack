package builder

import (
	"fmt"
	"strconv"

	"github.com/pkg/errors"

	"github.com/buildpack/pack/internal/api"
	"github.com/buildpack/pack/internal/dist"
	"github.com/buildpack/pack/internal/image"
	"github.com/buildpack/pack/internal/style"
)

type Image interface {
	BuildpackLayers() BuildpackLayers
	BuildpackAPIVersion() *api.Version
	BuildOnlyMixins() []string
	CommonMixins() []string
	GID() int
	Env(name string) (value string, err error)
	LifecycleVersion() *Version
	Metadata() Metadata
	Name() string
	Order() dist.Order
	PlatformAPIVersion() *api.Version
	StackID() string
	UID() int
}

//go:generate mockgen -package testmocks -destination testmocks/mock_build_image.go github.com/buildpack/pack/internal/builder BuildImage
type BuildImage interface {
	Name() string
	Label(name string) (value string, err error)
	Env(name string) (value string, err error)
	StackID() string
	CommonMixins() []string
	BuildOnlyMixins() []string
}

func NewImage(img BuildImage) (Image, error) {
	var metadata Metadata
	if _, err := image.UnmarshalLabel(img, metadataLabel, &metadata); err != nil {
		return nil, err
	}

	var order dist.Order
	if ok, err := image.UnmarshalLabel(img, orderLabel, &order); err != nil {
		return nil, err
	} else if !ok {
		order = metadata.Groups.ToOrder()
	}

	bpLayers := BuildpackLayers{}
	if _, err := image.UnmarshalLabel(img, buildpackLayersLabel, &bpLayers); err != nil {
		return nil, err
	}

	lifecycleVersion := VersionMustParse(AssumedLifecycleVersion)
	if metadata.Lifecycle.Version != nil {
		lifecycleVersion = metadata.Lifecycle.Version
	}

	buildpackAPIVersion := api.MustParse(dist.AssumedBuildpackAPIVersion)
	if metadata.Lifecycle.API.BuildpackVersion != nil {
		buildpackAPIVersion = metadata.Lifecycle.API.BuildpackVersion
	}

	platformAPIVersion := api.MustParse(AssumedPlatformAPIVersion)
	if metadata.Lifecycle.API.PlatformVersion != nil {
		platformAPIVersion = metadata.Lifecycle.API.PlatformVersion
	}

	uid, gid, err := extractUserAndGroupIDs(img)
	if err != nil {
		return nil, err
	}

	return &builderImage{
		img:                 img,
		uid:                 uid,
		gid:                 gid,
		lifecycleVersion:    lifecycleVersion,
		buildpackAPIVersion: buildpackAPIVersion,
		platformAPIVersion:  platformAPIVersion,
		metadata:            metadata,
		order:               order,
		bpLayers:            bpLayers,
	}, nil
}

// TODO: Test this
type builderImage struct {
	img                 BuildImage
	uid, gid            int
	metadata            Metadata
	order               dist.Order
	bpLayers            BuildpackLayers
	lifecycleVersion    *Version
	buildpackAPIVersion *api.Version
	platformAPIVersion  *api.Version
}

func (i *builderImage) BuildpackAPIVersion() *api.Version {
	return i.buildpackAPIVersion
}

func (i *builderImage) LifecycleVersion() *Version {
	return i.lifecycleVersion
}

func (i *builderImage) PlatformAPIVersion() *api.Version {
	return i.platformAPIVersion
}

func (i *builderImage) Name() string {
	return i.img.Name()
}

func (i *builderImage) GID() int {
	return i.gid
}

func (i *builderImage) UID() int {
	return i.uid
}

func (i *builderImage) Env(name string) (string, error) {
	return i.img.Env(name)
}

func (i *builderImage) Label(name string) (string, error) {
	return i.img.Label(name)
}

func (i *builderImage) StackID() string {
	return i.img.StackID()
}

func (i *builderImage) CommonMixins() []string {
	return i.img.CommonMixins()
}

func (i *builderImage) BuildOnlyMixins() []string {
	return i.img.BuildOnlyMixins()
}

func (i *builderImage) Metadata() Metadata {
	return i.metadata
}

func (i *builderImage) Order() dist.Order {
	return i.order
}

func (i *builderImage) BuildpackLayers() BuildpackLayers {
	return i.bpLayers
}

func extractUserAndGroupIDs(img BuildImage) (uid int, gid int, err error) {
	sUID, err := img.Env(envUID)
	if err != nil {
		return 0, 0, errors.Wrap(err, "reading builder env variables")
	} else if sUID == "" {
		return 0, 0, fmt.Errorf("image %s missing required env var %s", style.Symbol(img.Name()), style.Symbol(envUID))
	}

	sGID, err := img.Env(envGID)
	if err != nil {
		return 0, 0, errors.Wrap(err, "reading builder env variables")
	} else if sGID == "" {
		return 0, 0, fmt.Errorf("image %s missing required env var %s", style.Symbol(img.Name()), style.Symbol(envGID))
	}

	uid, err = strconv.Atoi(sUID)
	if err != nil {
		return 0, 0, fmt.Errorf("failed to parse %s, value %s should be an integer", style.Symbol(envUID), style.Symbol(sUID))
	}

	gid, err = strconv.Atoi(sGID)
	if err != nil {
		return 0, 0, fmt.Errorf("failed to parse %s, value %s should be an integer", style.Symbol(envGID), style.Symbol(sGID))
	}

	return uid, gid, nil
}
