package builder

import (
	"archive/tar"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strconv"
	"time"

	"github.com/BurntSushi/toml"
	"github.com/buildpack/imgutil"
	"github.com/pkg/errors"

	"github.com/buildpack/pack/api"
	"github.com/buildpack/pack/cmd"
	"github.com/buildpack/pack/dist"
	"github.com/buildpack/pack/internal/archive"
	"github.com/buildpack/pack/logging"
	"github.com/buildpack/pack/style"
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

	metadataLabel = "io.buildpacks.builder.metadata"
	stackLabel    = "io.buildpacks.stack.id"

	envUID = "CNB_USER_ID"
	envGID = "CNB_GROUP_ID"
)

type Builder struct {
	image                imgutil.Image
	lifecycle            Lifecycle
	lifecycleDescriptor  LifecycleDescriptor
	additionalBuildpacks []dist.Buildpack
	metadata             Metadata
	env                  map[string]string
	UID, GID             int
	StackID              string
	replaceOrder         bool
	order                dist.Order
}

type orderTOML struct {
	Order dist.Order `toml:"order"`
}

// GetBuilder constructs builder from builder image
func GetBuilder(img imgutil.Image) (*Builder, error) {
	var metadata Metadata
	if ok, err := dist.GetLabel(img, metadataLabel, &metadata); err != nil {
		return nil, err
	} else if !ok {
		return nil, fmt.Errorf("builder %s missing label %s -- try recreating builder", style.Symbol(img.Name()), style.Symbol(metadataLabel))
	}
	return constructBuilder(img, "", metadata)
}

// New constructs a new builder from base image
func New(baseImage imgutil.Image, name string) (*Builder, error) {
	var metadata Metadata
	if _, err := dist.GetLabel(baseImage, metadataLabel, &metadata); err != nil {
		return nil, err
	}
	return constructBuilder(baseImage, name, metadata)
}

func constructBuilder(img imgutil.Image, newName string, metadata Metadata) (*Builder, error) {
	uid, gid, err := userAndGroupIDs(img)
	if err != nil {
		return nil, err
	}

	stackID, err := img.Label(stackLabel)
	if err != nil {
		return nil, errors.Wrapf(err, "get label %s from image %s", style.Symbol(stackLabel), style.Symbol(img.Name()))
	}
	if stackID == "" {
		return nil, fmt.Errorf("image %s missing label %s", style.Symbol(img.Name()), style.Symbol(stackLabel))
	}

	if newName != "" && img.Name() != newName {
		img.Rename(newName)
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

	var order dist.Order
	if _, err := dist.GetLabel(img, OrderLabel, &order); err != nil {
		return nil, err
	}

	return &Builder{
		image:    img,
		metadata: metadata,
		order:    order,
		UID:      uid,
		GID:      gid,
		StackID:  stackID,
		lifecycleDescriptor: LifecycleDescriptor{
			Info: LifecycleInfo{
				Version: lifecycleVersion,
			},
			API: LifecycleAPI{
				PlatformVersion:  platformAPIVersion,
				BuildpackVersion: buildpackAPIVersion,
			},
		},
		env: map[string]string{},
	}, nil
}

func (b *Builder) Description() string {
	return b.metadata.Description
}

func (b *Builder) GetLifecycleDescriptor() LifecycleDescriptor {
	return b.lifecycleDescriptor
}

func (b *Builder) GetBuildpacks() []BuildpackMetadata {
	return b.metadata.Buildpacks
}

func (b *Builder) GetCreatedBy() CreatorMetadata {
	return b.metadata.CreatedBy
}

func (b *Builder) GetOrder() dist.Order {
	return b.order
}

func (b *Builder) Name() string {
	return b.image.Name()
}

func (b *Builder) GetStackInfo() StackMetadata {
	return b.metadata.Stack
}

func (b *Builder) AddBuildpack(bp dist.Buildpack) {
	b.additionalBuildpacks = append(b.additionalBuildpacks, bp)
	b.metadata.Buildpacks = append(b.metadata.Buildpacks, BuildpackMetadata{
		BuildpackInfo: bp.Descriptor().Info,
	})
}

func (b *Builder) SetLifecycle(lifecycle Lifecycle) error {
	b.lifecycle = lifecycle
	b.lifecycleDescriptor = lifecycle.Descriptor()
	return nil
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

func (b *Builder) SetStackInfo(stackConfig StackConfig) {
	b.metadata.Stack = StackMetadata{
		RunImage: RunImageMetadata{
			Image:   stackConfig.RunImage,
			Mirrors: stackConfig.RunImageMirrors,
		},
	}
}

func (b *Builder) Save(logger logging.Logger) error {
	resolvedOrder, err := processOrder(b.metadata.Buildpacks, b.order)
	if err != nil {
		return errors.Wrap(err, "processing order")
	}

	processMetadata(&b.metadata)

	tmpDir, err := ioutil.TempDir("", "create-builder-scratch")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmpDir)

	dirsTar, err := b.defaultDirsLayer(tmpDir)
	if err != nil {
		return err
	}
	if err := b.image.AddLayer(dirsTar); err != nil {
		return errors.Wrap(err, "adding default dirs layer")
	}

	if b.lifecycle != nil {
		b.metadata.Lifecycle.LifecycleInfo = b.lifecycle.Descriptor().Info
		b.metadata.Lifecycle.API = b.lifecycle.Descriptor().API
		lifecycleTar, err := b.lifecycleLayer(tmpDir)
		if err != nil {
			return err
		}
		if err := b.image.AddLayer(lifecycleTar); err != nil {
			return errors.Wrap(err, "adding lifecycle layer")
		}
	}

	if err := validateBuildpacks(b.StackID, b.GetLifecycleDescriptor(), b.additionalBuildpacks); err != nil {
		return errors.Wrap(err, "validating buildpacks")
	}

	bpLayers := BuildpackLayers{}
	if _, err := dist.GetLabel(b.image, BuildpackLayersLabel, &bpLayers); err != nil {
		return err
	}

	for _, bp := range b.additionalBuildpacks {
		bpLayerTar, err := dist.BuildpackLayer(tmpDir, b.UID, b.GID, bp)
		if err != nil {
			return err
		}

		if err := b.image.AddLayer(bpLayerTar); err != nil {
			return errors.Wrapf(err, "adding layer tar for buildpack %s:%s", style.Symbol(bp.Descriptor().Info.ID), style.Symbol(bp.Descriptor().Info.Version))
		}

		sha, err := sha256ForFile(bpLayerTar)
		if err != nil {
			return errors.Wrapf(err, "generating sha for %s", style.Symbol(bpLayerTar))
		}

		bpInfo := bp.Descriptor().Info
		if _, ok := bpLayers[bpInfo.ID]; !ok {
			bpLayers[bpInfo.ID] = map[string]BuildpackLayerInfo{}
		}

		if _, ok := bpLayers[bpInfo.ID][bpInfo.Version]; ok {
			logger.Warnf(
				"buildpack %s already exists on builder and will be overridden",
				style.Symbol(bpInfo.ID+"@"+bpInfo.Version),
			)
		}

		bpLayers[bpInfo.ID][bpInfo.Version] = BuildpackLayerInfo{
			LayerDigest: "sha256:" + sha,
			Order:       bp.Descriptor().Order,
		}
	}

	if err := dist.SetLabel(b.image, BuildpackLayersLabel, bpLayers); err != nil {
		return err
	}

	if b.replaceOrder {
		orderTar, err := b.orderLayer(resolvedOrder, tmpDir)
		if err != nil {
			return err
		}
		if err := b.image.AddLayer(orderTar); err != nil {
			return errors.Wrap(err, "adding order.tar layer")
		}

		if err := dist.SetLabel(b.image, OrderLabel, b.order); err != nil {
			return err
		}
	}

	stackTar, err := b.stackLayer(tmpDir)
	if err != nil {
		return err
	}
	if err := b.image.AddLayer(stackTar); err != nil {
		return errors.Wrap(err, "adding stack.tar layer")
	}

	compatTar, err := b.compatLayer(resolvedOrder, tmpDir)
	if err != nil {
		return err
	}

	if err := b.image.AddLayer(compatTar); err != nil {
		return errors.Wrap(err, "adding compat.tar layer")
	}

	envTar, err := b.envLayer(tmpDir, b.env)
	if err != nil {
		return err
	}
	if err := b.image.AddLayer(envTar); err != nil {
		return errors.Wrap(err, "adding env layer")
	}

	b.metadata.CreatedBy = CreatorMetadata{
		Name:    packName,
		Version: cmd.Version,
	}

	if err := dist.SetLabel(b.image, metadataLabel, b.metadata); err != nil {
		return err
	}

	if err := b.image.SetWorkingDir(layersDir); err != nil {
		return errors.Wrap(err, "failed to set working dir")
	}

	_, err = b.image.Save()
	return err
}

func sha256ForFile(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", errors.Wrap(err, "failed to open file")
	}
	hasher := sha256.New()
	if _, err := io.Copy(hasher, file); err != nil {
		return "", errors.Wrap(err, "failed to copy file to hasher")
	}

	return hex.EncodeToString(hasher.Sum(make([]byte, 0, hasher.Size()))), nil
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

func validateBuildpacks(stackID string, lifecycleDescriptor LifecycleDescriptor, bps []dist.Buildpack) error {
	bpLookup := map[string]interface{}{}

	for _, bp := range bps {
		bpInfo := bp.Descriptor().Info
		bpLookup[bpInfo.ID+"@"+bpInfo.Version] = nil
	}

	for _, bp := range bps {
		bpd := bp.Descriptor()

		if !bpd.API.SupportsVersion(lifecycleDescriptor.API.BuildpackVersion) {
			return fmt.Errorf(
				"buildpack %s (Buildpack API version %s) is incompatible with lifecycle %s (Buildpack API version %s)",
				style.Symbol(bpd.Info.ID+"@"+bpd.Info.Version),
				bpd.API.String(),
				style.Symbol(lifecycleDescriptor.Info.Version.String()),
				lifecycleDescriptor.API.BuildpackVersion.String(),
			)
		}

		if len(bpd.Stacks) >= 1 { // standard buildpack
			if !bpd.SupportsStack(stackID) {
				return fmt.Errorf(
					"buildpack %s does not support stack %s",
					style.Symbol(bpd.Info.ID+"@"+bpd.Info.Version),
					style.Symbol(stackID),
				)
			}
		} else { // order buildpack
			for _, g := range bpd.Order {
				for _, r := range g.Group {
					if _, ok := bpLookup[r.ID+"@"+r.Version]; !ok {
						return fmt.Errorf(
							"buildpack %s not found on the builder",
							style.Symbol(r.ID+"@"+r.Version),
						)
					}
				}
			}
		}
	}

	return nil
}

func userAndGroupIDs(img imgutil.Image) (int, int, error) {
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

	var uid, gid int
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
