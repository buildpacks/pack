package fakes

import (
	"archive/tar"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/go-containerregistry/pkg/name"
	"github.com/pkg/errors"

	"github.com/buildpack/imgutil"
)

func NewImage(name, topLayerSha string, identifier imgutil.Identifier) *Image {
	return &Image{
		labels:        map[string]string{},
		env:           map[string]string{},
		topLayerSha:   topLayerSha,
		identifier:    identifier,
		name:          name,
		cmd:           []string{"initialCMD"},
		layersMap:     map[string]string{},
		prevLayersMap: map[string]string{},
		createdAt:     time.Now(),
		savedNames:    map[string]bool{},
	}
}

type Image struct {
	deleted       bool
	layers        []string
	layersMap     map[string]string
	prevLayersMap map[string]string
	reusedLayers  []string
	labels        map[string]string
	env           map[string]string
	topLayerSha   string
	identifier    imgutil.Identifier
	name          string
	entryPoint    []string
	cmd           []string
	base          string
	createdAt     time.Time
	layerDir      string
	workingDir    string
	savedNames    map[string]bool
}

func (i *Image) CreatedAt() (time.Time, error) {
	return i.createdAt, nil
}

func (i *Image) Label(key string) (string, error) {
	return i.labels[key], nil
}

func (i *Image) Rename(name string) {
	i.name = name
}

func (i *Image) Name() string {
	return i.name
}

func (i *Image) Identifier() (imgutil.Identifier, error) {
	return i.identifier, nil
}

func (i *Image) Rebase(baseTopLayer string, newBase imgutil.Image) error {
	i.base = newBase.Name()
	return nil
}

func (i *Image) SetLabel(k string, v string) error {
	i.labels[k] = v
	return nil
}

func (i *Image) SetEnv(k string, v string) error {
	i.env[k] = v
	return nil
}

func (i *Image) SetWorkingDir(dir string) error {
	i.workingDir = dir
	return nil
}

func (i *Image) SetEntrypoint(v ...string) error {
	i.entryPoint = v
	return nil
}

func (i *Image) SetCmd(v ...string) error {
	i.cmd = v
	return nil
}

func (i *Image) Env(k string) (string, error) {
	return i.env[k], nil
}

func (i *Image) TopLayer() (string, error) {
	return i.topLayerSha, nil
}

func (i *Image) AddLayer(path string) error {
	sha, err := shaForFile(path)
	if err != nil {
		return err
	}

	i.layersMap["sha256:"+sha] = path
	i.layers = append(i.layers, path)
	return nil
}

func shaForFile(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", errors.Wrapf(err, "failed to open file")
	}

	hasher := sha256.New()
	if _, err := io.Copy(hasher, file); err != nil {
		return "", errors.Wrapf(err, "failed to copy file to hasher")
	}

	return hex.EncodeToString(hasher.Sum(make([]byte, 0, hasher.Size()))), nil
}

func (i *Image) GetLayer(sha string) (io.ReadCloser, error) {
	path, ok := i.layersMap[sha]
	if !ok {
		return nil, fmt.Errorf("failed to get layer with sha '%s'", sha)
	}

	return os.Open(path)
}

func (i *Image) ReuseLayer(sha string) error {
	prevLayer, ok := i.prevLayersMap[sha]
	if !ok {
		return fmt.Errorf("image does not have previous layer with sha '%s'", sha)
	}
	i.reusedLayers = append(i.reusedLayers, sha)
	i.layersMap[sha] = prevLayer
	return nil
}

func (i *Image) Save(additionalNames ...string) error {
	var err error
	i.layerDir, err = ioutil.TempDir("", "fake-image")
	if err != nil {
		return err
	}

	for sha, path := range i.layersMap {
		newPath := filepath.Join(i.layerDir, filepath.Base(path))
		i.copyLayer(path, newPath)
		i.layersMap[sha] = newPath
	}

	for l := range i.layers {
		layerPath := i.layers[l]
		i.layers[l] = filepath.Join(i.layerDir, filepath.Base(layerPath))
	}

	allNames := append([]string{i.name}, additionalNames...)

	var errs []imgutil.SaveDiagnostic
	for _, n := range allNames {
		_, err := name.ParseReference(n, name.WeakValidation)
		if err != nil {
			errs = append(errs, imgutil.SaveDiagnostic{ImageName: n, Cause: err})
		} else {
			i.savedNames[n] = true
		}
	}

	if len(errs) > 0 {
		return imgutil.SaveError{Errors: errs}
	}

	return nil
}

func (i *Image) copyLayer(path, newPath string) error {
	src, err := os.Open(path)
	if err != nil {
		return errors.Wrap(err, "opening layer during copy")
	}
	defer src.Close()

	dst, err := os.Create(newPath)
	if err != nil {
		return errors.Wrap(err, "creating new layer during copy")
	}
	defer dst.Close()

	if _, err := io.Copy(dst, src); err != nil {
		return errors.Wrap(err, "copying layers")
	}

	return nil
}

func (i *Image) Delete() error {
	i.deleted = true
	return nil
}

func (i *Image) Found() bool {
	return !i.deleted
}

// test methods

func (i *Image) SetIdentifier(identifier imgutil.Identifier) {
	i.identifier = identifier
}

func (i *Image) Cleanup() error {
	return os.RemoveAll(i.layerDir)
}

func (i *Image) AppLayerPath() string {
	return i.layers[0]
}

func (i *Image) Entrypoint() ([]string, error) {
	return i.entryPoint, nil
}

func (i *Image) Cmd() ([]string, error) {
	return i.cmd, nil
}

func (i *Image) ConfigLayerPath() string {
	return i.layers[1]
}

func (i *Image) ReusedLayers() []string {
	return i.reusedLayers
}

func (i *Image) WorkingDir() string {
	return i.workingDir
}

func (i *Image) AddPreviousLayer(sha, path string) {
	i.prevLayersMap[sha] = path
}

func (i *Image) FindLayerWithPath(path string) (string, error) {
	// we iterate backwards over the layer array b/c later layers could replace a file with a given path
	for idx := len(i.layers) - 1; idx >= 0; idx-- {
		tarPath := i.layers[idx]
		r, _ := os.Open(tarPath)
		defer r.Close()

		tr := tar.NewReader(r)
		for {
			header, err := tr.Next()
			if err == io.EOF {
				break
			} else if err != nil {
				return "", errors.Wrap(err, "finding next header in layer")
			}

			if header.Name == path {
				return tarPath, nil
			}
		}
	}
	return "", fmt.Errorf("Could not find %s in any layer. \n \n %s", path, i.tarContents())
}

func (i *Image) tarContents() string {
	var strBuilder = strings.Builder{}
	for _, tarPath := range i.layers {
		strBuilder.WriteString(fmt.Sprintf("layer %s --- \n Contents: \n", filepath.Base(tarPath)))

		r, _ := os.Open(tarPath)
		defer r.Close()

		tr := tar.NewReader(r)

		for {
			header, err := tr.Next()
			if err == io.EOF {
				break
			}

			if header.Typeflag != tar.TypeDir {
				strBuilder.WriteString(fmt.Sprintf("%s \n", header.Name))
			}
		}
		strBuilder.WriteString("\n \n")
	}
	return strBuilder.String()
}

func (i *Image) NumberOfAddedLayers() int {
	return len(i.layers)
}

func (i *Image) IsSaved() bool {
	return len(i.savedNames) > 0
}

func (i *Image) Base() string {
	return i.base
}

func (i *Image) SavedNames() []string {
	var names []string
	for k := range i.savedNames {
		names = append(names, k)
	}

	return names
}
