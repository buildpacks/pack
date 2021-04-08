package fakes

import (
	"fmt"
	"io"
	"os"
)

type FakeReadable struct {
	layers        []string
	layersMap     map[string]string
	labels        map[string]string
}

func NewFakeReadable() *FakeReadable {
	return &FakeReadable{}
}

func (f *FakeReadable) SetLabel(label, contents string) {
	f.labels[label] = contents
}

func (f *FakeReadable) SetLayer(label, contents string) {
	f.labels[label] = contents
}

func (f *FakeReadable) AddLayerWithDiffID(path string, diffID string) error {
	f.layersMap[diffID] = path
	f.layers = append(f.layers, path)
	return nil
}

func (f *FakeReadable) GetLayer(sha string) (io.ReadCloser, error) {
	path, ok := f.layersMap[sha]
	if !ok {
		return nil, fmt.Errorf("failed to get layer with sha '%s'", sha)
	}

	return os.Open(path)
}

func (f *FakeReadable) Label(key string) (string, error) {
	return f.labels[key], nil
}
