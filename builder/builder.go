package builder

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
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
	"github.com/Masterminds/semver"
	"github.com/buildpack/imgutil"
	"github.com/pkg/errors"

	"github.com/buildpack/pack/buildpack"
	"github.com/buildpack/pack/internal/archive"
	"github.com/buildpack/pack/lifecycle"
	"github.com/buildpack/pack/style"
)

const (
	buildpacksDir = "/buildpacks"
	platformDir   = "/platform"
	lifecycleDir  = "/lifecycle"
	workspaceDir  = "/workspace"
	layersDir     = "/layers"
	stackLabel    = "io.buildpacks.stack.id"
	envUID        = "CNB_USER_ID"
	envGID        = "CNB_GROUP_ID"
)

type Builder struct {
	image         imgutil.Image
	lifecyclePath string
	buildpacks    []buildpack.Buildpack
	metadata      Metadata
	env           map[string]string
	UID, GID      int
	StackID       string
	replaceOrder  bool
	order         Order
}

type orderTOML struct {
	Order Order `toml:"order"`
}

type Order []OrderEntry

type OrderEntry struct {
	Group []BuildpackRef `toml:"group"`
}

type BuildpackRef struct {
	buildpack.BuildpackInfo
	Optional bool `toml:"optional,omitempty"`
}

func GetBuilder(img imgutil.Image) (*Builder, error) {
	uid, gid, err := userAndGroupIDs(img)
	if err != nil {
		return nil, err
	}

	stackID, err := img.Label("io.buildpacks.stack.id")
	if err != nil {
		return nil, errors.Wrapf(err, "get label %s from image %s", style.Symbol(stackLabel), style.Symbol(img.Name()))
	} else if stackID == "" {
		return nil, fmt.Errorf("image %s missing %s' label'", style.Symbol(img.Name()), style.Symbol(stackLabel))
	}

	label, err := img.Label(MetadataLabel)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to find run images for builder %s", style.Symbol(img.Name()))
	} else if label == "" {
		return nil, fmt.Errorf("builder %s missing label %s -- try recreating builder", style.Symbol(img.Name()), style.Symbol(MetadataLabel))
	}

	var metadata Metadata
	if err := json.Unmarshal([]byte(label), &metadata); err != nil {
		return nil, errors.Wrapf(err, "failed to parse metadata for builder %s", style.Symbol(img.Name()))
	}

	return &Builder{
		image:    img,
		metadata: metadata,
		UID:      uid,
		GID:      gid,
		StackID:  stackID,
	}, nil
}

func (b *Builder) Description() string {
	return b.metadata.Description
}

func (b *Builder) GetLifecycleVersion() *semver.Version {
	return b.metadata.Lifecycle.Version
}

func (b *Builder) GetBuildpacks() []BuildpackMetadata {
	return b.metadata.Buildpacks
}

// TODO: change to v2 order type when order label is added
func (b *Builder) GetOrder() []V1Group {
	return b.metadata.Groups
}

func (b *Builder) Name() string {
	return b.image.Name()
}

func (b *Builder) GetStackInfo() StackMetadata {
	return b.metadata.Stack
}

func New(img imgutil.Image, name string) (*Builder, error) {
	uid, gid, err := userAndGroupIDs(img)
	if err != nil {
		return nil, err
	}

	stackID, err := img.Label(stackLabel)
	if err != nil {
		return nil, errors.Wrapf(err, "get label %s from image '%s'", style.Symbol(stackLabel), style.Symbol(img.Name()))
	}
	if stackID == "" {
		return nil, fmt.Errorf("image %s missing %s label", style.Symbol(img.Name()), style.Symbol(stackLabel))
	}

	label, err := img.Label(MetadataLabel)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get metadata label from image %s", style.Symbol(img.Name()))
	} else if label == "" {
		label = "{}"
	}

	var metadata Metadata
	if err := json.Unmarshal([]byte(label), &metadata); err != nil {
		return nil, errors.Wrapf(err, "failed to parse metadata for builder %s", style.Symbol(img.Name()))
	}

	img.Rename(name)
	return &Builder{
		image:    img,
		UID:      uid,
		GID:      gid,
		StackID:  stackID,
		metadata: metadata,
		env:      map[string]string{},
	}, nil
}

func (b *Builder) AddBuildpack(bp buildpack.Buildpack) {
	b.buildpacks = append(b.buildpacks, bp)
	b.metadata.Buildpacks = append(b.metadata.Buildpacks, BuildpackMetadata{
		BuildpackInfo: bp.BuildpackInfo,
	})
}

func (b *Builder) SetLifecycle(md lifecycle.Metadata) error {
	b.metadata.Lifecycle.Version = md.Version
	b.lifecyclePath = md.Path
	return nil
}

func (b *Builder) SetEnv(env map[string]string) {
	b.env = env
}

func (b *Builder) SetOrder(order Order) {
	b.metadata.Groups = order.ToV1Order()
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
	if err := processMetadata(&b.metadata); err != nil {
		return errors.Wrap(err, "processing metadata")
	}

	if err := validateBuildpacks(b.StackID, b.buildpacks); err != nil {
		return errors.Wrap(err, "validating buildpacks")
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

	envTar, err := b.envLayer(tmpDir, b.env)
	if err != nil {
		return err
	}
	if err := b.image.AddLayer(envTar); err != nil {
		return errors.Wrap(err, "adding env layer")
	}

	if b.lifecyclePath != "" {
		lifecycleTar, err := b.lifecycleLayer(tmpDir)
		if err != nil {
			return err
		}
		if err := b.image.AddLayer(lifecycleTar); err != nil {
			return errors.Wrap(err, "adding lifecycle layer")
		}
	}

	for _, bp := range b.buildpacks {
		layerTar, err := b.buildpackLayer(tmpDir, bp)
		if err != nil {
			return err
		}
		if err := b.image.AddLayer(layerTar); err != nil {
			return errors.Wrapf(err, "adding layer tar for buildpack %s:%s", style.Symbol(bp.ID), style.Symbol(bp.Version))
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

	_, err = b.image.Save()
	return err
}

// TODO: error out when using incompatible lifecycle and buildpacks
func validateBuildpacks(stackID string, bps []buildpack.Buildpack) error {
	bpLookup := map[string]interface{}{}

	for _, bp := range bps {
		bpLookup[bp.ID+"@"+bp.Version] = nil
	}

	for _, bp := range bps {
		if len(bp.Order) == 0 && len(bp.Stacks) == 0 {
			return fmt.Errorf("buildpack %s must have either stacks or an order defined", style.Symbol(bp.ID+"@"+bp.Version))
		}

		if len(bp.Order) >= 1 && len(bp.Stacks) >= 1 {
			return fmt.Errorf("buildpack %s cannot have both stacks and an order defined", style.Symbol(bp.ID+"@"+bp.Version))
		}

		if len(bp.Stacks) >= 1 && !bp.SupportsStack(stackID) {
			return fmt.Errorf(
				"buildpack %s does not support stack %s",
				style.Symbol(bp.ID+"@"+bp.Version), style.Symbol(stackID),
			)
		}

		for _, g := range bp.Order {
			for _, r := range g.Group {
				if _, ok := bpLookup[r.ID+"@"+r.Version]; !ok {
					return fmt.Errorf("buildpack %s not found on the builder", style.Symbol(r.ID+"@"+r.Version))
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
	buf := &bytes.Buffer{}
	lifecycleVersion := b.GetLifecycleVersion()

	var tomlData interface{}
	if lifecycleVersion != nil && lifecycleVersion.LessThan(semver.MustParse("0.4.0")) {
		tomlData = v1OrderTOML{Groups: b.metadata.Groups}
	} else {
		tomlData = orderTOML{Order: b.order}
	}

	err := toml.NewEncoder(buf).Encode(tomlData)
	if err != nil {
		return "", errors.Wrapf(err, "failed to marshal order.toml")
	}

	layerTar := filepath.Join(dest, "order.tar")
	err = archive.CreateSingleFileTar(layerTar, path.Join(buildpacksDir, "order.toml"), buf.String())
	if err != nil {
		return "", errors.Wrapf(err, "failed to create order.toml layer tar")
	}

	return layerTar, nil
}

func (b *Builder) stackLayer(dest string) (string, error) {
	buf := &bytes.Buffer{}
	err := toml.NewEncoder(buf).Encode(b.metadata.Stack)
	if err != nil {
		return "", errors.Wrapf(err, "failed to marshal stack.toml")
	}

	layerTar := filepath.Join(dest, "stack.tar")
	err = archive.CreateSingleFileTar(layerTar, path.Join(buildpacksDir, "stack.toml"), buf.String())
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
func (b *Builder) buildpackLayer(dest string, bp buildpack.Buildpack) (string, error) {
	layerTar := filepath.Join(dest, fmt.Sprintf("%s.%s.tar", bp.EscapedID(), bp.Version))

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
		Name:     path.Join(buildpacksDir, bp.EscapedID()),
		Mode:     0755,
		ModTime:  now,
	}); err != nil {
		return "", err
	}

	baseTarDir := path.Join(buildpacksDir, bp.EscapedID(), bp.Version)
	if err := tw.WriteHeader(&tar.Header{
		Typeflag: tar.TypeDir,
		Name:     baseTarDir,
		Mode:     0755,
		ModTime:  now,
	}); err != nil {
		return "", err
	}

	if filepath.Ext(bp.Path) == ".tgz" {
		err = b.embedBuildpackTar(tw, bp.Path, baseTarDir)
	} else {
		err = archive.WriteDirToTar(
			tw,
			bp.Path,
			baseTarDir,
			b.UID,
			b.GID,
			-1,
		)
	}

	if err != nil {
		return "", errors.Wrapf(err, "creating layer tar for buildpack '%s:%s'", bp.ID, bp.Version)
	}

	return layerTar, nil
}

func (b *Builder) embedBuildpackTar(tw *tar.Writer, srcTar, baseTarDir string) error {
	var (
		tarFile    *os.File
		gzipReader *gzip.Reader
		fhFinal    io.Reader
		err        error
	)

	tarFile, err = os.Open(srcTar)
	fhFinal = tarFile
	if err != nil {
		return errors.Wrapf(err, "failed to open buildpack tar '%s'", srcTar)
	}
	defer tarFile.Close()

	gzipReader, err = gzip.NewReader(tarFile)
	fhFinal = gzipReader
	if err != nil {
		return errors.Wrap(err, "failed to create gzip reader")
	}

	defer gzipReader.Close()

	tr := tar.NewReader(fhFinal)
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

func (b *Builder) embedLifecycleTar(tw *tar.Writer, srcTar string) error {
	var (
		tarFile    *os.File
		gzipReader *gzip.Reader
		fhFinal    io.Reader
		err        error
		regex      = regexp.MustCompile(`^[^/]+/([^/]+)$`)
	)

	tarFile, err = os.Open(srcTar)
	fhFinal = tarFile
	if err != nil {
		return errors.Wrapf(err, "failed to open lifecycle tar '%s'", srcTar)
	}
	defer tarFile.Close()

	gzipReader, err = gzip.NewReader(tarFile)
	fhFinal = gzipReader
	if err != nil {
		return errors.Wrap(err, "failed to create gzip reader")
	}

	defer gzipReader.Close()

	tr := tar.NewReader(fhFinal)
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

	err = b.embedLifecycleTar(tw, b.lifecyclePath)
	if err != nil {
		return "", err
	}

	return fh.Name(), nil
}
