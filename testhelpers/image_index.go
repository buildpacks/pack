package testhelpers

import (
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/buildpacks/imgutil"
	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/random"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/google/go-containerregistry/pkg/v1/types"
)

func AssertRemoteImageIndex(t *testing.T, repoName string, mediaType types.MediaType, expectedNumberOfManifests int) {
	t.Helper()

	remoteIndex := FetchImageIndexDescriptor(t, repoName)
	AssertNotNil(t, remoteIndex)
	remoteIndexMediaType, err := remoteIndex.MediaType()
	AssertNil(t, err)
	AssertEq(t, remoteIndexMediaType, mediaType)
	remoteIndexManifest, err := remoteIndex.IndexManifest()
	AssertNil(t, err)
	AssertNotNil(t, remoteIndexManifest)
	AssertEq(t, len(remoteIndexManifest.Manifests), expectedNumberOfManifests)
}

func AssertPathExists(t *testing.T, path string) {
	t.Helper()
	_, err := os.Stat(path)
	if os.IsNotExist(err) {
		t.Errorf("Expected %q to exist", path)
	} else if err != nil {
		t.Fatalf("Error stating %q: %v", path, err)
	}
}

func FetchImageIndexDescriptor(t *testing.T, repoName string) v1.ImageIndex {
	t.Helper()

	r, err := name.ParseReference(repoName, name.WeakValidation)
	AssertNil(t, err)

	auth, err := authn.DefaultKeychain.Resolve(r.Context().Registry)
	AssertNil(t, err)

	index, err := remote.Index(r, remote.WithTransport(http.DefaultTransport), remote.WithAuth(auth))
	AssertNil(t, err)

	return index
}

func ReadIndexManifest(t *testing.T, path string) *v1.IndexManifest {
	t.Helper()

	indexPath := filepath.Join(path, "index.json")
	AssertPathExists(t, filepath.Join(path, "oci-layout"))
	AssertPathExists(t, indexPath)

	// check index file
	data, err := os.ReadFile(indexPath)
	AssertNil(t, err)

	index := &v1.IndexManifest{}
	err = json.Unmarshal(data, index)
	AssertNil(t, err)
	return index
}

func RandomCNBIndex(t *testing.T, repoName string, layers, count int64) *imgutil.CNBIndex {
	t.Helper()

	randomIndex, err := random.Index(1024, layers, count)
	AssertNil(t, err)
	options := &imgutil.IndexOptions{
		BaseIndex: randomIndex,
		LayoutIndexOptions: imgutil.LayoutIndexOptions{
			XdgPath: os.Getenv("XDG_RUNTIME_DIR"),
		},
	}
	idx, err := imgutil.NewCNBIndex(repoName, *options)
	AssertNil(t, err)
	return idx
}

// MockImageIndex wraps a real CNBIndex to record if some key methods are invoke
type MockImageIndex struct {
	imgutil.CNBIndex
	ErrorOnSave     bool
	PushCalled      bool
	DeleteDirCalled bool
}

// NewMockImageIndex creates a random index with the given number of layers and manifests count
func NewMockImageIndex(t *testing.T, repoName string, layers, count int64) *MockImageIndex {
	cnbIdx := RandomCNBIndex(t, repoName, layers, count)
	idx := &MockImageIndex{
		CNBIndex: *cnbIdx,
	}
	return idx
}

func (i *MockImageIndex) SaveDir() error {
	if i.ErrorOnSave {
		return errors.New("something failed writing the index on disk")
	}
	return i.CNBIndex.SaveDir()
}

func (i *MockImageIndex) Push(_ ...imgutil.IndexOption) error {
	i.PushCalled = true
	return nil
}

func (i *MockImageIndex) DeleteDir() error {
	i.DeleteDirCalled = true
	return nil
}
