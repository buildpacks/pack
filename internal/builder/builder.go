package builder

import (
	"archive/tar"
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"time"

	"github.com/BurntSushi/toml"
	"github.com/buildpack/imgutil"
	"github.com/pkg/errors"

	"github.com/buildpack/pack/builder"
	"github.com/buildpack/pack/cmd"
	"github.com/buildpack/pack/internal/archive"
	"github.com/buildpack/pack/internal/dist"
	"github.com/buildpack/pack/internal/image"
	"github.com/buildpack/pack/internal/stack"
	"github.com/buildpack/pack/internal/stringset"
	"github.com/buildpack/pack/internal/style"
	"github.com/buildpack/pack/logging"
)

const (
	packName = "Pack CLI"

	cnbDir = "/cnb"

	orderPath    = "/cnb/order.toml"
	stackPath    = "/cnb/stack.toml"
	platformDir  = "/platform"
	lifecycleDir = "/cnb/lifecycle"
	workspaceDir = "/workspace"
	layersDir    = "/layers"

	metadataLabel        = "io.buildpacks.builder.metadata"
	orderLabel           = "io.buildpacks.buildpack.order"
	buildpackLayersLabel = "io.buildpacks.buildpack.layers"

	envUID = "CNB_USER_ID"
	envGID = "CNB_GROUP_ID"
)

type ImageFactory interface {
	NewImage(repoName string, daemon bool) (imgutil.Image, error)
}

type Builder struct {
	lifecycle            Lifecycle
	lifecycleDescriptor  LifecycleDescriptor
	buildpackLayers      BuildpackLayers
	additionalBuildpacks []dist.Buildpack
	metadata             Metadata
	mixins               []string
	env                  map[string]string
	UID, GID             int
	StackID              string
	replaceOrder         bool
	order                dist.Order
}

type orderTOML struct {
	Order dist.Order `toml:"order"`
}

type ImageStore interface {
	Name() string
	Rename(name string)
	Label(string) (string, error)
	Env(name string) (value string, err error)
	SetLabel(name string, value string) error
	AddLayer(path string) error
	SetWorkingDir(path string) error
	Save(additionalNames ...string) error
}

// FromBuilderImage constructs a builder from an existing image
func FromBuilderImage(builderImage Image) (*Builder, error) {
	bldr := &Builder{
		lifecycleDescriptor: LifecycleDescriptor{
			Info: LifecycleInfo{
				Version: builderImage.LifecycleVersion(),
			},
			API: LifecycleAPI{
				BuildpackVersion: builderImage.BuildpackAPIVersion(),
				PlatformVersion:  builderImage.PlatformAPIVersion(),
			},
		},
		buildpackLayers:      builderImage.BuildpackLayers(),
		additionalBuildpacks: nil,
		// baseImageName: baseName,  // TODO: investigate
		metadata: builderImage.Metadata(),
		mixins:   stringset.Join(builderImage.CommonMixins(), builderImage.BuildOnlyMixins()),
		env:      map[string]string{},
		UID:      builderImage.UID(),
		GID:      builderImage.GID(),
		StackID:  builderImage.StackID(),
		order:    builderImage.Order(),
	}

	return bldr, nil
}

func (b *Builder) Description() string {
	return b.metadata.Description
}

func (b *Builder) LifecycleDescriptor() LifecycleDescriptor {
	return b.lifecycleDescriptor
}

func (b *Builder) Buildpacks() []BuildpackMetadata {
	return b.metadata.Buildpacks
}

func (b *Builder) CreatedBy() CreatorMetadata {
	return b.metadata.CreatedBy
}

func (b *Builder) Order() dist.Order {
	return b.order
}
func (b *Builder) Stack() StackMetadata {
	return b.metadata.Stack
}

func (b *Builder) Mixins() []string {
	return b.mixins
}

func (b *Builder) AddBuildpack(bp dist.Buildpack) {
	b.additionalBuildpacks = append(b.additionalBuildpacks, bp)
	b.metadata.Buildpacks = append(b.metadata.Buildpacks, BuildpackMetadata{
		BuildpackInfo: bp.Descriptor().Info,
	})
}

func (b *Builder) SetLifecycle(lifecycle Lifecycle) {
	b.lifecycle = lifecycle
	b.lifecycleDescriptor = lifecycle.Descriptor()
}

func (b *Builder) SetEnv(env map[string]string) {
	b.env = env
}

func (b *Builder) SetOrder(order dist.Order) {
	b.order = order
	b.replaceOrder = true
}

func (b *Builder) SetDescription(description string) {
	b.metadata.Description = description
}

func (b *Builder) SetStack(stackConfig builder.StackConfig) {
	b.metadata.Stack = StackMetadata{
		RunImage: RunImageMetadata{
			Image:   stackConfig.RunImage,
			Mirrors: stackConfig.RunImageMirrors,
		},
	}
}

func (b *Builder) Save(logger logging.Logger, store ImageStore) (Image, error) {
	resolvedOrder, err := processOrder(b.metadata.Buildpacks, b.order)
	if err != nil {
		return nil, errors.Wrap(err, "processing order")
	}

	b.metadata.Groups = orderToV1Order(resolvedOrder)
	processMetadata(&b.metadata)

	tmpDir, err := ioutil.TempDir("", "create-builder-scratch")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(tmpDir)

	dirsTar, err := b.defaultDirsLayer(tmpDir)
	if err != nil {
		return nil, err
	}

	if err := store.AddLayer(dirsTar); err != nil {
		return nil, errors.Wrap(err, "adding default dirs layer")
	}

	if b.lifecycle != nil {
		b.metadata.Lifecycle.LifecycleInfo = b.lifecycle.Descriptor().Info
		b.metadata.Lifecycle.API = b.lifecycle.Descriptor().API
		lifecycleTar, err := b.lifecycleLayer(tmpDir)
		if err != nil {
			return nil, err
		}
		if err := store.AddLayer(lifecycleTar); err != nil {
			return nil, errors.Wrap(err, "adding lifecycle layer")
		}
	}

	if err := validateBuildpacks(b.StackID, b.Mixins(), b.LifecycleDescriptor(), b.additionalBuildpacks); err != nil {
		return nil, errors.Wrap(err, "validating buildpacks")
	}

	bpLayers := b.buildpackLayers

	for _, bp := range b.additionalBuildpacks {
		bpLayerTar, err := dist.BuildpackLayer(tmpDir, b.UID, b.GID, bp)
		if err != nil {
			return nil, err
		}

		if err := store.AddLayer(bpLayerTar); err != nil {
			return nil, errors.Wrapf(err,
				"adding layer tar for buildpack %s",
				style.Symbol(bp.Descriptor().Info.FullName()),
			)
		}

		diffID, err := dist.LayerDiffID(bpLayerTar)
		if err != nil {
			return nil, errors.Wrapf(err,
				"getting content hashes for buildpack %s",
				style.Symbol(bp.Descriptor().Info.FullName()),
			)
		}

		bpInfo := bp.Descriptor().Info
		if _, ok := bpLayers[bpInfo.ID]; !ok {
			bpLayers[bpInfo.ID] = map[string]BuildpackLayerInfo{}
		}

		if _, ok := bpLayers[bpInfo.ID][bpInfo.Version]; ok {
			logger.Warnf(
				"buildpack %s already exists on builder and will be overridden",
				style.Symbol(bpInfo.FullName()),
			)
		}

		bpLayers[bpInfo.ID][bpInfo.Version] = BuildpackLayerInfo{
			LayerDiffID: diffID.String(),
			Order:       bp.Descriptor().Order,
			Stacks:      bp.Descriptor().Stacks,
		}
	}

	if err := image.MarshalToLabel(store, buildpackLayersLabel, bpLayers); err != nil {
		return nil, err
	}

	if b.replaceOrder {
		orderTar, err := b.orderLayer(resolvedOrder, tmpDir)
		if err != nil {
			return nil, err
		}
		if err := store.AddLayer(orderTar); err != nil {
			return nil, errors.Wrap(err, "adding order.tar layer")
		}

		if err := image.MarshalToLabel(store, orderLabel, b.order); err != nil {
			return nil, err
		}
	}

	stackTar, err := b.stackLayer(tmpDir)
	if err != nil {
		return nil, err
	}
	if err := store.AddLayer(stackTar); err != nil {
		return nil, errors.Wrap(err, "adding stack.tar layer")
	}

	compatTar, err := b.compatLayer(resolvedOrder, tmpDir)
	if err != nil {
		return nil, err
	}

	if err := store.AddLayer(compatTar); err != nil {
		return nil, errors.Wrap(err, "adding compat.tar layer")
	}

	envTar, err := b.envLayer(tmpDir, b.env)
	if err != nil {
		return nil, err
	}
	if err := store.AddLayer(envTar); err != nil {
		return nil, errors.Wrap(err, "adding env layer")
	}

	b.metadata.CreatedBy = CreatorMetadata{
		Name:    packName,
		Version: cmd.Version,
	}

	if err := image.MarshalToLabel(store, metadataLabel, b.metadata); err != nil {
		return nil, err
	}

	if err := store.SetWorkingDir(layersDir); err != nil {
		return nil, errors.Wrap(err, "failed to set working dir")
	}

	if err = store.Save(); err != nil {
		return nil, err
	}

	stackImage, err := stack.NewImage(store)
	if err != nil {
		return nil, err
	}

	buildImage, err := stack.NewBuildImage(stackImage)
	if err != nil {
		return nil, err
	}

	return &builderImage{
		img:      buildImage,
		metadata: Metadata{},
		order:    nil,
		bpLayers: nil,
	}, nil
}

func processOrder(buildpacks []BuildpackMetadata, order dist.Order) (dist.Order, error) {
	resolvedOrder := dist.Order{}

	for gi, g := range order {
		resolvedOrder = append(resolvedOrder, dist.OrderEntry{})

		for _, bpRef := range g.Group {
			var matchingBps []dist.BuildpackInfo
			for _, bp := range buildpacks {
				if bpRef.ID == bp.ID {
					matchingBps = append(matchingBps, bp.BuildpackInfo)
				}
			}

			if len(matchingBps) == 0 {
				return dist.Order{}, fmt.Errorf("no versions of buildpack %s were found on the builder", style.Symbol(bpRef.ID))
			}

			if bpRef.Version == "" {
				if len(matchingBps) > 1 {
					return dist.Order{}, fmt.Errorf("unable to resolve version: multiple versions of %s - must specify an explicit version", style.Symbol(bpRef.ID))
				}

				bpRef.Version = matchingBps[0].Version
			}

			if !hasBuildpackWithVersion(matchingBps, bpRef.Version) {
				return dist.Order{}, fmt.Errorf("buildpack %s with version %s was not found on the builder", style.Symbol(bpRef.ID), style.Symbol(bpRef.Version))
			}

			resolvedOrder[gi].Group = append(resolvedOrder[gi].Group, bpRef)
		}
	}

	return resolvedOrder, nil
}

func hasBuildpackWithVersion(bps []dist.BuildpackInfo, version string) bool {
	for _, bp := range bps {
		if bp.Version == version {
			return true
		}
	}
	return false
}

func validateBuildpacks(stackID string, mixins []string, lifecycleDescriptor LifecycleDescriptor, bps []dist.Buildpack) error {
	bpLookup := map[string]interface{}{}

	for _, bp := range bps {
		bpLookup[bp.Descriptor().Info.FullName()] = nil
	}

	for _, bp := range bps {
		bpd := bp.Descriptor()

		if !bpd.API.SupportsVersion(lifecycleDescriptor.API.BuildpackVersion) {
			return fmt.Errorf(
				"buildpack %s (Buildpack API version %s) is incompatible with lifecycle %s (Buildpack API version %s)",
				style.Symbol(bpd.Info.FullName()),
				bpd.API.String(),
				style.Symbol(lifecycleDescriptor.Info.Version.String()),
				lifecycleDescriptor.API.BuildpackVersion.String(),
			)
		}

		if len(bpd.Stacks) >= 1 { // standard buildpack
			if err := bpd.EnsureStackSupport(stackID, mixins, false); err != nil {
				return err
			}
		} else { // order buildpack
			for _, g := range bpd.Order {
				for _, r := range g.Group {
					if _, ok := bpLookup[r.FullName()]; !ok {
						return fmt.Errorf(
							"buildpack %s not found on the builder",
							style.Symbol(r.FullName()),
						)
					}
				}
			}
		}
	}

	return nil
}

func (b *Builder) defaultDirsLayer(dest string) (string, error) {
	fh, err := os.Create(filepath.Join(dest, "dirs.tar"))
	if err != nil {
		return "", err
	}
	defer fh.Close()

	tw := tar.NewWriter(fh)
	defer tw.Close()

	ts := archive.NormalizedDateTime

	if err := tw.WriteHeader(b.packOwnedDir(workspaceDir, ts)); err != nil {
		return "", errors.Wrapf(err, "creating %s dir in layer", style.Symbol(workspaceDir))
	}

	if err := tw.WriteHeader(b.packOwnedDir(layersDir, ts)); err != nil {
		return "", errors.Wrapf(err, "creating %s dir in layer", style.Symbol(layersDir))
	}

	if err := tw.WriteHeader(b.rootOwnedDir(cnbDir, ts)); err != nil {
		return "", errors.Wrapf(err, "creating %s dir in layer", style.Symbol(cnbDir))
	}

	if err := tw.WriteHeader(b.rootOwnedDir(dist.BuildpacksDir, ts)); err != nil {
		return "", errors.Wrapf(err, "creating %s dir in layer", style.Symbol(dist.BuildpacksDir))
	}

	if err := tw.WriteHeader(b.rootOwnedDir(platformDir, ts)); err != nil {
		return "", errors.Wrapf(err, "creating %s dir in layer", style.Symbol(platformDir))
	}

	if err := tw.WriteHeader(b.rootOwnedDir(platformDir+"/env", ts)); err != nil {
		return "", errors.Wrapf(err, "creating %s dir in layer", style.Symbol(platformDir+"/env"))
	}

	return fh.Name(), nil
}

func (b *Builder) packOwnedDir(path string, time time.Time) *tar.Header {
	return &tar.Header{
		Typeflag: tar.TypeDir,
		Name:     path,
		Mode:     0755,
		ModTime:  time,
		Uid:      b.UID,
		Gid:      b.GID,
	}
}

func (b *Builder) rootOwnedDir(path string, time time.Time) *tar.Header {
	return &tar.Header{
		Typeflag: tar.TypeDir,
		Name:     path,
		Mode:     0755,
		ModTime:  time,
	}
}

func (b *Builder) orderLayer(order dist.Order, dest string) (string, error) {
	contents, err := orderFileContents(order)
	if err != nil {
		return "", err
	}

	layerTar := filepath.Join(dest, "order.tar")
	err = archive.CreateSingleFileTar(layerTar, orderPath, contents)
	if err != nil {
		return "", errors.Wrapf(err, "failed to create order.toml layer tar")
	}

	return layerTar, nil
}

func orderFileContents(order dist.Order) (string, error) {
	buf := &bytes.Buffer{}

	tomlData := orderTOML{Order: order}
	if err := toml.NewEncoder(buf).Encode(tomlData); err != nil {
		return "", errors.Wrapf(err, "failed to marshal order.toml")
	}
	return buf.String(), nil
}

func (b *Builder) stackLayer(dest string) (string, error) {
	buf := &bytes.Buffer{}
	err := toml.NewEncoder(buf).Encode(b.metadata.Stack)
	if err != nil {
		return "", errors.Wrapf(err, "failed to marshal stack.toml")
	}

	layerTar := filepath.Join(dest, "stack.tar")
	err = archive.CreateSingleFileTar(layerTar, stackPath, buf.String())
	if err != nil {
		return "", errors.Wrapf(err, "failed to create stack.toml layer tar")
	}

	return layerTar, nil
}

func (b *Builder) embedLifecycleTar(tw *tar.Writer) error {
	var regex = regexp.MustCompile(`^[^/]+/([^/]+)$`)

	lr, err := b.lifecycle.Open()
	if err != nil {
		return errors.Wrap(err, "failed to open lifecycle")
	}
	defer lr.Close()
	tr := tar.NewReader(lr)
	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return errors.Wrap(err, "failed to get next tar entry")
		}

		pathMatches := regex.FindStringSubmatch(path.Clean(header.Name))
		if pathMatches != nil {
			binaryName := pathMatches[1]

			header.Name = lifecycleDir + "/" + binaryName
			err = tw.WriteHeader(header)
			if err != nil {
				return errors.Wrapf(err, "failed to write header for '%s'", header.Name)
			}

			buf, err := ioutil.ReadAll(tr)
			if err != nil {
				return errors.Wrapf(err, "failed to read contents of '%s'", header.Name)
			}

			_, err = tw.Write(buf)
			if err != nil {
				return errors.Wrapf(err, "failed to write contents to '%s'", header.Name)
			}
		}
	}

	return nil
}

func (b *Builder) envLayer(dest string, env map[string]string) (string, error) {
	fh, err := os.Create(filepath.Join(dest, "env.tar"))
	if err != nil {
		return "", err
	}
	defer fh.Close()

	tw := tar.NewWriter(fh)
	defer tw.Close()

	for k, v := range env {
		if err := tw.WriteHeader(&tar.Header{
			Name:    path.Join(platformDir, "env", k),
			Size:    int64(len(v)),
			Mode:    0644,
			ModTime: archive.NormalizedDateTime,
		}); err != nil {
			return "", err
		}
		if _, err := tw.Write([]byte(v)); err != nil {
			return "", err
		}
	}

	return fh.Name(), nil
}

func (b *Builder) lifecycleLayer(dest string) (string, error) {
	fh, err := os.Create(filepath.Join(dest, "lifecycle.tar"))
	if err != nil {
		return "", err
	}
	defer fh.Close()

	tw := tar.NewWriter(fh)
	defer tw.Close()

	if err := tw.WriteHeader(&tar.Header{
		Typeflag: tar.TypeDir,
		Name:     lifecycleDir,
		Mode:     0755,
		ModTime:  archive.NormalizedDateTime,
	}); err != nil {
		return "", err
	}

	err = b.embedLifecycleTar(tw)
	if err != nil {
		return "", errors.Wrap(err, "embedding lifecycle tar")
	}

	return fh.Name(), nil
}
