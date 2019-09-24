package builder

import (
	"archive/tar"
	"bytes"
	"encoding/json"
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
	"github.com/buildpack/pack/internal/archive"
	"github.com/buildpack/pack/style"
)

const (
	cnbDir        = "/cnb"
	buildpacksDir = "/cnb/buildpacks"
	orderPath     = "/cnb/order.toml"
	stackPath     = "/cnb/stack.toml"
	platformDir   = "/platform"
	lifecycleDir  = "/cnb/lifecycle"
	workspaceDir  = "/workspace"
	layersDir     = "/layers"
	stackLabel    = "io.buildpacks.stack.id"
	envUID        = "CNB_USER_ID"
	envGID        = "CNB_GROUP_ID"
)

type Builder struct {
	image                imgutil.Image
	lifecycle            Lifecycle
	lifecycleDescriptor  LifecycleDescriptor
	additionalBuildpacks []Buildpack
	metadata             Metadata
	env                  map[string]string
	UID, GID             int
	StackID              string
	replaceOrder         bool
	order                Order
}

type orderTOML struct {
	Order Order `toml:"order"`
}

type Order []OrderEntry

type OrderEntry struct {
	Group []BuildpackRef `toml:"group"`
}

type BuildpackRef struct {
	BuildpackInfo
	Optional bool `toml:"optional,omitempty"`
}

// GetBuilder constructs builder from builder image
func GetBuilder(img imgutil.Image) (*Builder, error) {
	label, err := img.Label(MetadataLabel)
	if err != nil {
		return nil, errors.Wrapf(err, "get label %s from image %s", style.Symbol(MetadataLabel), style.Symbol(img.Name()))
	}
	if label == "" {
		return nil, fmt.Errorf("builder %s missing label %s -- try recreating builder", style.Symbol(img.Name()), style.Symbol(MetadataLabel))
	}

	return constructBuilder(img, "", label)
}

// New constructs a new builder from base image
func New(baseImage imgutil.Image, name string) (*Builder, error) {
	label, err := baseImage.Label(MetadataLabel)
	if err != nil {
		return nil, errors.Wrapf(err, "get label %s from image %s", style.Symbol(MetadataLabel), style.Symbol(baseImage.Name()))
	}

	return constructBuilder(baseImage, name, label)
}

func constructBuilder(img imgutil.Image, newName string, label string) (*Builder, error) {
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

	var metadata Metadata
	if label != "" {
		if err := json.Unmarshal([]byte(label), &metadata); err != nil {
			return nil, errors.Wrapf(err, "failed to parse metadata for builder %s", style.Symbol(img.Name()))
		}
	}

	if newName != "" && img.Name() != newName {
		img.Rename(newName)
	}

	lifecycleVersion := VersionMustParse(AssumedLifecycleVersion)
	if metadata.Lifecycle.Version != nil {
		lifecycleVersion = metadata.Lifecycle.Version
	}

	buildpackApiVersion := api.MustParse(AssumedBuildpackAPIVersion)
	if metadata.Lifecycle.API.BuildpackVersion != nil {
		buildpackApiVersion = metadata.Lifecycle.API.BuildpackVersion
	}

	platformApiVersion := api.MustParse(AssumedPlatformAPIVersion)
	if metadata.Lifecycle.API.PlatformVersion != nil {
		platformApiVersion = metadata.Lifecycle.API.PlatformVersion
	}

	return &Builder{
		image:    img,
		metadata: metadata,
		order:    metadata.Groups.ToOrder(),
		UID:      uid,
		GID:      gid,
		StackID:  stackID,
		lifecycleDescriptor: LifecycleDescriptor{
			Info: LifecycleInfo{
				Version: lifecycleVersion,
			},
			API: LifecycleAPI{
				PlatformVersion:  platformApiVersion,
				BuildpackVersion: buildpackApiVersion,
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

func (b *Builder) GetOrder() Order {
	return b.order
}

func (b *Builder) Name() string {
	return b.image.Name()
}

func (b *Builder) GetStackInfo() StackMetadata {
	return b.metadata.Stack
}

func (b *Builder) AddBuildpack(bp Buildpack) {
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

func (b *Builder) SetOrder(order Order) {
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

func (b *Builder) Save() error {
	if err := processOrder(b.metadata.Buildpacks, &b.order); err != nil {
		return errors.Wrap(err, "processing order")
	}

	b.metadata.Groups = b.order.ToV1Order()
	if err := processMetadata(&b.metadata); err != nil {
		return errors.Wrap(err, "processing metadata")
	}

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

	for _, bp := range b.additionalBuildpacks {
		bpLayerTar, err := b.buildpackLayer(tmpDir, bp)
		if err != nil {
			return err
		}
		if err := b.image.AddLayer(bpLayerTar); err != nil {
			return errors.Wrapf(err, "adding layer tar for buildpack %s:%s", style.Symbol(bp.Descriptor().Info.ID), style.Symbol(bp.Descriptor().Info.Version))
		}
	}

	if b.replaceOrder {
		orderTar, err := b.orderLayer(tmpDir)
		if err != nil {
			return err
		}
		if err := b.image.AddLayer(orderTar); err != nil {
			return errors.Wrap(err, "adding order.tar layer")
		}
	}

	stackTar, err := b.stackLayer(tmpDir)
	if err != nil {
		return err
	}
	if err := b.image.AddLayer(stackTar); err != nil {
		return errors.Wrap(err, "adding stack.tar layer")
	}

	compatTar, err := b.compatLayer(tmpDir)
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

	label, err := json.Marshal(b.metadata)
	if err != nil {
		return errors.Wrap(err, "failed marshal builder image metadata")
	}

	if err := b.image.SetLabel(MetadataLabel, string(label)); err != nil {
		return errors.Wrap(err, "failed to set metadata label")
	}

	if err := b.image.SetWorkingDir(layersDir); err != nil {
		return errors.Wrap(err, "failed to set working dir")
	}

	err = b.image.Save()
	return err
}

func processOrder(buildpacks []BuildpackMetadata, order *Order) error {
	for _, g := range *order {
		for i := range g.Group {
			bpRef := &g.Group[i]

			var matchingBps []BuildpackInfo
			for _, bp := range buildpacks {
				if bpRef.ID == bp.ID {
					matchingBps = append(matchingBps, bp.BuildpackInfo)
				}
			}

			if len(matchingBps) == 0 {
				return fmt.Errorf("no versions of buildpack %s were found on the builder", style.Symbol(bpRef.ID))
			}

			if bpRef.Version == "" {
				if len(matchingBps) > 1 {
					return fmt.Errorf("unable to resolve version: multiple versions of %s - must specify an explicit version", style.Symbol(bpRef.ID))
				}

				bpRef.Version = matchingBps[0].Version
			}

			if !hasBuildpackWithVersion(matchingBps, bpRef.Version) {
				return fmt.Errorf("buildpack %s with version %s was not found on the builder", style.Symbol(bpRef.ID), style.Symbol(bpRef.Version))
			}
		}
	}

	return nil
}

func hasBuildpackWithVersion(bps []BuildpackInfo, version string) bool {
	for _, bp := range bps {
		if bp.Version == version {
			return true
		}
	}
	return false
}

func validateBuildpacks(stackID string, lifecycleDescriptor LifecycleDescriptor, bps []Buildpack) error {
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

	now := time.Now()

	if err := tw.WriteHeader(b.packOwnedDir(workspaceDir, now)); err != nil {
		return "", errors.Wrapf(err, "creating %s dir in layer", style.Symbol(workspaceDir))
	}

	if err := tw.WriteHeader(b.packOwnedDir(layersDir, now)); err != nil {
		return "", errors.Wrapf(err, "creating %s dir in layer", style.Symbol(layersDir))
	}

	if err := tw.WriteHeader(b.rootOwnedDir(cnbDir, now)); err != nil {
		return "", errors.Wrapf(err, "creating %s dir in layer", style.Symbol(cnbDir))
	}

	if err := tw.WriteHeader(b.rootOwnedDir(buildpacksDir, now)); err != nil {
		return "", errors.Wrapf(err, "creating %s dir in layer", style.Symbol(buildpacksDir))
	}

	if err := tw.WriteHeader(b.rootOwnedDir(platformDir, now)); err != nil {
		return "", errors.Wrapf(err, "creating %s dir in layer", style.Symbol(platformDir))
	}

	if err := tw.WriteHeader(b.rootOwnedDir(platformDir+"/env", now)); err != nil {
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

func (b *Builder) orderLayer(dest string) (string, error) {
	contents, err := b.orderFileContents()
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

func (b *Builder) orderFileContents() (string, error) {
	buf := &bytes.Buffer{}
	bpAPIVersion := api.MustParse(AssumedBuildpackAPIVersion)
	if b.GetLifecycleDescriptor().Info.Version != nil {
		bpAPIVersion = b.GetLifecycleDescriptor().API.BuildpackVersion
	}

	var tomlData interface{}
	if bpAPIVersion.Equal(api.MustParse("0.1")) {
		tomlData = v1OrderTOML{Groups: b.metadata.Groups}
	} else {
		tomlData = orderTOML{Order: b.order}
	}
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

// Output:
//
// layer tar = {ID}.{V}.tar
//
// inside the layer = /buildpacks/{ID}/{V}/*
func (b *Builder) buildpackLayer(dest string, bp Buildpack) (string, error) {
	bpd := bp.Descriptor()
	layerTar := filepath.Join(dest, fmt.Sprintf("%s.%s.tar", bpd.EscapedID(), bpd.Info.Version))

	fh, err := os.Create(layerTar)
	if err != nil {
		return "", fmt.Errorf("create file for tar: %s", err)
	}
	defer fh.Close()

	tw := tar.NewWriter(fh)
	defer tw.Close()

	now := time.Now()

	if err := tw.WriteHeader(&tar.Header{
		Typeflag: tar.TypeDir,
		Name:     path.Join(buildpacksDir, bpd.EscapedID()),
		Mode:     0755,
		ModTime:  now,
	}); err != nil {
		return "", err
	}

	baseTarDir := path.Join(buildpacksDir, bpd.EscapedID(), bpd.Info.Version)
	if err := tw.WriteHeader(&tar.Header{
		Typeflag: tar.TypeDir,
		Name:     baseTarDir,
		Mode:     0755,
		ModTime:  now,
	}); err != nil {
		return "", err
	}

	if err := b.embedBuildpackTar(tw, bp, baseTarDir); err != nil {
		return "", errors.Wrapf(err, "creating layer tar for buildpack '%s:%s'", bpd.Info.ID, bpd.Info.Version)
	}

	return layerTar, nil
}

func (b *Builder) embedBuildpackTar(tw *tar.Writer, bp Buildpack, baseTarDir string) error {
	var (
		err error
	)

	rc, err := bp.Open()
	if err != nil {
		errors.Wrap(err, "read buildpack blob")
	}
	defer rc.Close()

	tr := tar.NewReader(rc)
	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return errors.Wrap(err, "failed to get next tar entry")
		}

		header.Name = path.Clean(header.Name)
		if header.Name == "." || header.Name == "/" {
			continue
		}

		header.Name = path.Clean(path.Join(baseTarDir, header.Name))
		header.Uid = b.UID
		header.Gid = b.GID
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

	return nil
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

	now := time.Now()

	for k, v := range env {
		if err := tw.WriteHeader(&tar.Header{
			Name:    path.Join(platformDir, "env", k),
			Size:    int64(len(v)),
			Mode:    0644,
			ModTime: now,
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

	now := time.Now()

	if err := tw.WriteHeader(&tar.Header{
		Typeflag: tar.TypeDir,
		Name:     lifecycleDir,
		Mode:     0755,
		ModTime:  now,
	}); err != nil {
		return "", err
	}

	err = b.embedLifecycleTar(tw)
	if err != nil {
		return "", errors.Wrap(err, "embedding lifecycle tar")
	}

	return fh.Name(), nil
}
