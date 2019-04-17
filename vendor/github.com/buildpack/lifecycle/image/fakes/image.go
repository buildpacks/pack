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
	"testing"
	"time"

	"github.com/buildpack/lifecycle/image"
)

func NewImage(t *testing.T, name, topLayerSha, digest string) *Image {
	return &Image{
		t:            t,
		alreadySaved: false,
		labels:       make(map[string]string),
		env:          make(map[string]string),
		topLayerSha:  topLayerSha,
		digest:       digest,
		name:         name,
		cmd:          []string{"initialCMD"},
		layersMap:    map[string]string{},
		createdAt:    time.Now(),
	}
}

type Image struct {
	t            *testing.T
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
}

func (f *Image) CreatedAt() (time.Time, error) {
	return f.createdAt, nil
}

func (f *Image) Label(key string) (string, error) {
	return f.labels[key], nil
}

func (f *Image) Rename(name string) {
	f.assertNotAlreadySaved()
	f.name = name
}

func (f *Image) Name() string {
	return f.name
}

func (f *Image) Digest() (string, error) {
	return f.digest, nil
}

func (f *Image) Rebase(baseTopLayer string, newBase image.Image) error {
	f.base = newBase.Name()
	return nil
}

func (f *Image) SetLabel(k string, v string) error {
	f.assertNotAlreadySaved()
	f.labels[k] = v
	return nil
}

func (f *Image) SetEnv(k string, v string) error {
	f.assertNotAlreadySaved()
	f.env[k] = v
	return nil
}

func (f *Image) SetEntrypoint(v ...string) error {
	f.assertNotAlreadySaved()
	f.entryPoint = v
	return nil
}

func (f *Image) SetCmd(v ...string) error {
	f.assertNotAlreadySaved()
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
	f.assertNotAlreadySaved()

	f.layersMap["sha256:"+shaForFile(f.t, path)] = path
	f.layers = append(f.layers, path)
	return nil
}

func shaForFile(t *testing.T, path string) string {
	t.Helper()

	file, err := os.Open(path)
	if err != nil {
		t.Fatalf("failed to open file: %s", err)
	}
	hasher := sha256.New()
	if _, err := io.Copy(hasher, file); err != nil {
		t.Fatalf("failed to copy file to hasher: %s", err)
	}

	return hex.EncodeToString(hasher.Sum(make([]byte, 0, hasher.Size())))
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
	f.assertNotAlreadySaved()

	f.reusedLayers = append(f.reusedLayers, sha)
	return nil
}

func (f *Image) Save() (string, error) {
	f.assertNotAlreadySaved()
	f.alreadySaved = true

	var err error
	f.layerDir, err = ioutil.TempDir("", "fake-image")
	if err != nil {
		f.t.Fatalf("failed to create tmpDir: %s", err)
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

func (f *Image) copyLayer(path, newPath string) {
	src, err := os.Open(path)
	if err != nil {
		f.t.Fatal(err)
	}
	defer src.Close()

	dst, err := os.Create(newPath)
	if err != nil {
		f.t.Fatal(err)
	}
	defer dst.Close()

	if _, err := io.Copy(dst, src); err != nil {
		f.t.Fatal(err)
	}
}

func (f *Image) Delete() error {
	f.deleted = true
	return nil
}

func (f *Image) Found() (bool, error) {
	return !f.deleted, nil
}

// test methods

func (f *Image) Cleanup() {
	if err := os.RemoveAll(f.layerDir); err != nil {
		f.t.Fatal(err)
	}
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

func (f *Image) FindLayerWithPath(path string) string {
	f.t.Helper()

	for _, tarPath := range f.layersMap {

		r, _ := os.Open(tarPath)
		defer r.Close()

		tr := tar.NewReader(r)
		for {
			header, err := tr.Next()
			if err == io.EOF {
				break
			} else if err != nil {
				f.t.Fatal(err)
			}

			if header.Name == path {
				return tarPath
			}
		}
	}

	f.t.Fatalf("Could not find %s in any layer. \n \n %s", path, f.tarContents())
	return ""
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

func (f *Image) assertNotAlreadySaved() {
	f.t.Helper()
	if f.alreadySaved {
		f.t.Fatalf("Image has already been saved")
	}
}

func (f *Image) IsSaved() bool {
	return f.alreadySaved
}

func (f *Image) Base() string {
	return f.base
}
