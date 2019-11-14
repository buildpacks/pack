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
	ReadableImage
	StackID() string
	CommonMixins() []string
	BuildOnlyMixins() []string
}

type ReadableImage interface {
	Name() string
	Label(name string) (value string, err error)
	Env(name string) (value string, err error)
}

type builderInfo struct {
	metadata *Metadata
	order    *dist.Order
	bpLayers *BuildpackLayers
	gid, uid int
}

func NewImage(img BuildImage) (Image, error) {
	info, err := extractBuilderInfo(img, true)
	if err != nil {
		return nil, err
	}

	if info.metadata.Stack.RunImage.Image == "" {
		return nil, errors.New("builder metadata is missing runImage")
	}

	return &builderImage{
		img:                 img,
		name:                img.Name(),
		commonMixins:        img.CommonMixins(),
		buildOnlyMixins:     img.BuildOnlyMixins(),
		uid:                 info.uid,
		gid:                 info.gid,
		metadata:            *info.metadata,
		order:               *info.order,
		bpLayers:            *info.bpLayers,
		lifecycleVersion:    info.metadata.Lifecycle.Version,
		buildpackAPIVersion: info.metadata.Lifecycle.API.BuildpackVersion,
		platformAPIVersion:  info.metadata.Lifecycle.API.PlatformVersion,
		stackID:             img.StackID(),
	}, nil
}

func extractBuilderInfo(img ReadableImage, strict bool) (*builderInfo, error) {
	metadata := &Metadata{}
	if ok, err := image.UnmarshalLabel(img, metadataLabel, metadata); err != nil {
		return nil, err
	} else if !ok && strict {
		return nil, errors.Errorf("missing label %s", style.Symbol(metadataLabel))
	}

	order := &dist.Order{}
	if ok, err := image.UnmarshalLabel(img, orderLabel, order); err != nil {
		return nil, err
	} else if !ok {
		o := metadata.Groups.ToOrder()
		order = &o
	}

	bpLayers := &BuildpackLayers{}
	if ok, err := image.UnmarshalLabel(img, buildpackLayersLabel, bpLayers); err != nil {
		return nil, err
	} else if !ok && strict {
		return nil, errors.Errorf("missing label %s", style.Symbol(buildpackLayersLabel))
	}

	uid, gid, err := extractUserAndGroupIDs(img)
	if err != nil {
		return nil, err
	}

	if metadata.Lifecycle.Version == nil {
		metadata.Lifecycle.Version = VersionMustParse(AssumedLifecycleVersion)
	}

	if metadata.Lifecycle.API.BuildpackVersion == nil {
		metadata.Lifecycle.API.BuildpackVersion = api.MustParse(dist.AssumedBuildpackAPIVersion)
	}

	if metadata.Lifecycle.API.PlatformVersion == nil {
		metadata.Lifecycle.API.PlatformVersion = api.MustParse(AssumedPlatformAPIVersion)
	}

	return &builderInfo{
		metadata: metadata,
		order:    order,
		bpLayers: bpLayers,
		uid:      uid,
		gid:      gid,
	}, nil
}

// TODO: Test this
type builderImage struct {
	img                 BuildImage // Deprecated: should store data instead of delegating
	name                string
	commonMixins        []string
	buildOnlyMixins     []string
	uid, gid            int
	metadata            Metadata
	order               dist.Order
	bpLayers            BuildpackLayers
	lifecycleVersion    *Version
	buildpackAPIVersion *api.Version
	platformAPIVersion  *api.Version
	stackID             string
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
	return i.name
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
	return i.stackID
}

func (i *builderImage) CommonMixins() []string {
	return i.commonMixins
}

func (i *builderImage) BuildOnlyMixins() []string {
	return i.buildOnlyMixins
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

func extractUserAndGroupIDs(img ReadableImage) (uid int, gid int, err error) {
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
