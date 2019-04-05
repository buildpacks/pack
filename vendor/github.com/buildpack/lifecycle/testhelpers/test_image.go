package testhelpers

import (
	"archive/tar"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/buildpack/lifecycle/image"
)

func NewFakeImage(t *testing.T, name, topLayerSha, digest string) *FakeImage {
	return &FakeImage{
		t:            t,
		alreadySaved: false,
		labels:       make(map[string]string),
		env:          make(map[string]string),
		topLayerSha:  topLayerSha,
		digest:       digest,
		name:         name,
		cmd:          []string{"initialCMD"},
		layersMap:    map[string]string{},
	}
}

type FakeImage struct {
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
}

func (f *FakeImage) Label(key string) (string, error) {
	return f.labels[key], nil
}

func (f *FakeImage) Rename(name string) {
	f.assertNotAlreadySaved()
	f.name = name
}

func (f *FakeImage) Name() string {
	return f.name
}

func (f *FakeImage) Digest() (string, error) {
	return f.digest, nil
}

func (f *FakeImage) Rebase(baseTopLayer string, newBase image.Image) error {
	f.base = newBase.Name()
	return nil
}

func (f *FakeImage) SetLabel(k string, v string) error {
	f.assertNotAlreadySaved()
	f.labels[k] = v
	return nil
}

func (f *FakeImage) SetEnv(k string, v string) error {
	f.assertNotAlreadySaved()
	f.env[k] = v
	return nil
}

func (f *FakeImage) SetEntrypoint(v ...string) error {
	f.assertNotAlreadySaved()
	f.entryPoint = v
	return nil
}

func (f *FakeImage) SetCmd(v ...string) error {
	f.assertNotAlreadySaved()
	f.cmd = v
	return nil
}

func (f *FakeImage) Env(k string) (string, error) {
	return f.env[k], nil
}

func (f *FakeImage) TopLayer() (string, error) {
	return f.topLayerSha, nil
}

func (f *FakeImage) AddLayer(path string) error {
	f.assertNotAlreadySaved()

	f.layersMap["sha256:"+ComputeSHA256ForFile(f.t, path)] = path
	f.layers = append(f.layers, path)
	return nil
}

func (f *FakeImage) GetLayer(sha string) (io.ReadCloser, error) {
	path, ok := f.layersMap[sha]
	if !ok {
		f.t.Fatalf("failed to get layer with sha '%s'", sha)
	}
	return os.Open(path)
}

func (f *FakeImage) ReuseLayer(sha string) error {
	f.assertNotAlreadySaved()

	f.reusedLayers = append(f.reusedLayers, sha)
	return nil
}

func (f *FakeImage) Save() (string, error) {
	f.assertNotAlreadySaved()
	f.alreadySaved = true
	return "saved-digest-from-fake-run-image", nil
}

func (f *FakeImage) Delete() error {
	f.deleted = true
	return nil
}

func (f *FakeImage) Found() (bool, error) {
	return !f.deleted, nil
}

// test methods

func (f *FakeImage) AppLayerPath() string {
	return f.layers[0]
}

func (f *FakeImage) Entrypoint() ([]string, error) {
	return f.entryPoint, nil
}

func (f *FakeImage) Cmd() ([]string, error) {
	return f.cmd, nil
}

func (f *FakeImage) ConfigLayerPath() string {
	return f.layers[1]
}

func (f *FakeImage) ReusedLayers() []string {
	return f.reusedLayers
}

func (f *FakeImage) FindLayerWithPath(path string) string {
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

func (f *FakeImage) tarContents() string {
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

func (f *FakeImage) NumberOfAddedLayers() int {
	return len(f.layers)
}

func (f *FakeImage) assertNotAlreadySaved() {
	f.t.Helper()
	if f.alreadySaved {
		f.t.Fatalf("Image has already been saved")
	}
}

func (f *FakeImage) IsSaved() bool {
	return f.alreadySaved
}

func (f *FakeImage) Base() string {
	return f.base
}
