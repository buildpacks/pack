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

	"github.com/pkg/errors"

	"github.com/buildpack/imgutil"
)

func NewImage(name, topLayerSha, digest string) *Image {
	return &Image{
		alreadySaved: false,
		labels:       map[string]string{},
		env:          map[string]string{},
		topLayerSha:  topLayerSha,
		digest:       digest,
		name:         name,
		cmd:          []string{"initialCMD"},
		layersMap:    map[string]string{},
		createdAt:    time.Now(),
	}
}

type Image struct {
	alreadySaved bool
	deleted      bool
	layers       []string
	layersMap    map[string]string
	reusedLayers []string
	labels       map[string]string
	env          map[string]string
	topLayerSha  string
	digest       string
	name         string
	entryPoint   []string
	cmd          []string
	base         string
	createdAt    time.Time
	layerDir     string
	workingDir   string
}

func (f *Image) CreatedAt() (time.Time, error) {
	return f.createdAt, nil
}

func (f *Image) Label(key string) (string, error) {
	return f.labels[key], nil
}

func (f *Image) Rename(name string) {
	f.name = name
}

func (f *Image) Name() string {
	return f.name
}

func (f *Image) Digest() (string, error) {
	return f.digest, nil
}

func (f *Image) Rebase(baseTopLayer string, newBase imgutil.Image) error {
	f.base = newBase.Name()
	return nil
}

func (f *Image) SetLabel(k string, v string) error {
	f.labels[k] = v
	return nil
}

func (f *Image) SetEnv(k string, v string) error {
	f.env[k] = v
	return nil
}

func (f *Image) SetWorkingDir(dir string) error {
	f.workingDir = dir
	return nil
}

func (f *Image) SetEntrypoint(v ...string) error {
	f.entryPoint = v
	return nil
}

func (f *Image) SetCmd(v ...string) error {
	f.cmd = v
	return nil
}

func (f *Image) Env(k string) (string, error) {
	return f.env[k], nil
}

func (f *Image) TopLayer() (string, error) {
	return f.topLayerSha, nil
}

func (f *Image) AddLayer(path string) error {
	sha, err := shaForFile(path)
	if err != nil {
		return err
	}

	f.layersMap["sha256:"+sha] = path
	f.layers = append(f.layers, path)
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

func (f *Image) GetLayer(sha string) (io.ReadCloser, error) {
	for _, s := range f.reusedLayers {
		if s == sha {
			return ioutil.NopCloser(strings.NewReader("dummy data")), nil
		}
	}

	path, ok := f.layersMap[sha]
	if !ok {
		return nil, fmt.Errorf("failed to get layer with sha '%s'", sha)
	}

	return os.Open(path)
}

func (f *Image) ReuseLayer(sha string) error {
	f.reusedLayers = append(f.reusedLayers, sha)
	return nil
}

func (f *Image) Save() (string, error) {
	f.alreadySaved = true

	var err error
	f.layerDir, err = ioutil.TempDir("", "fake-image")
	if err != nil {
		return "", errors.Wrap(err, "failed to create tmpDir")
	}

	for sha, path := range f.layersMap {
		newPath := filepath.Join(f.layerDir, filepath.Base(path))
		f.copyLayer(path, newPath)
		f.layersMap[sha] = newPath
	}

	for i := range f.layers {
		layerPath := f.layers[i]
		f.layers[i] = filepath.Join(f.layerDir, filepath.Base(layerPath))
	}

	return "saved-digest-from-fake-run-image", nil
}

func (f *Image) copyLayer(path, newPath string) error {
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

func (f *Image) Delete() error {
	f.deleted = true
	return nil
}

func (f *Image) Found() (bool, error) {
	return !f.deleted, nil
}

// test methods

func (f *Image) Cleanup() error {
	return os.RemoveAll(f.layerDir)
}

func (f *Image) AppLayerPath() string {
	return f.layers[0]
}

func (f *Image) Entrypoint() ([]string, error) {
	return f.entryPoint, nil
}

func (f *Image) Cmd() ([]string, error) {
	return f.cmd, nil
}

func (f *Image) ConfigLayerPath() string {
	return f.layers[1]
}

func (f *Image) ReusedLayers() []string {
	return f.reusedLayers
}

func (f *Image) WorkingDir() string {
	return f.workingDir
}

func (f *Image) FindLayerWithPath(path string) (string, error) {
	// we iterate backwards over the layer array b/c later layers could replace a file with a given path
	for i := len(f.layers) - 1; i >= 0; i-- {
		tarPath := f.layers[i]
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
	return "", fmt.Errorf("Could not find %s in any layer. \n \n %s", path, f.tarContents())
}

func (f *Image) tarContents() string {
	var strBuilder = strings.Builder{}
	for _, tarPath := range f.layers {
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

func (f *Image) NumberOfAddedLayers() int {
	return len(f.layers)
}

func (f *Image) IsSaved() bool {
	return f.alreadySaved
}

func (f *Image) Base() string {
	return f.base
}
