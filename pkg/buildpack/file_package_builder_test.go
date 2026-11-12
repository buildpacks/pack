package buildpack_test

import (
	"archive/tar"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/buildpacks/lifecycle/api"
	"github.com/opencontainers/go-digest"
	v1 "github.com/opencontainers/image-spec/specs-go/v1"

	"github.com/buildpacks/pack/internal/fakes"
	"github.com/buildpacks/pack/pkg/blob"
	"github.com/buildpacks/pack/pkg/buildpack"
	"github.com/buildpacks/pack/pkg/dist"
)

func packageForTargets(t *testing.T, targets ...dist.Target) string {
	t.Helper()
	writer, err := buildpack.NewFilePackageBuilder()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := writer.Close(); err != nil {
			t.Error(err)
		}
	})
	for _, target := range targets {
		descriptor := &dist.BuildpackDescriptor{
			WithAPI:     api.MustParse("0.10"),
			WithInfo:    dist.ModuleInfo{ID: "example/multi", Version: "1.0"},
			WithTargets: []dist.Target{target},
		}
		content, err := fakes.NewFakeBuildpackBlob(descriptor, 0755)
		if err != nil {
			t.Fatal(err)
		}
		builder := buildpack.NewBuilder(nil)
		builder.SetBuildpack(buildpack.FromBlob(descriptor, content))
		if err := writer.Add(builder, target, nil); err != nil {
			t.Fatal(err)
		}
	}
	filename := filepath.Join(t.TempDir(), "multi.cnb")
	if err := writer.Save(filename); err != nil {
		t.Fatal(err)
	}
	return filename
}

func packageEntries(t *testing.T, filename string) map[string][]byte {
	t.Helper()
	f, err := os.Open(filename)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	entries := make(map[string][]byte)
	reader := tar.NewReader(f)
	for {
		header, err := reader.Next()
		if err == io.EOF {
			return entries
		}
		if err != nil {
			t.Fatal(err)
		}
		if header.Typeflag != tar.TypeReg {
			continue
		}
		name := strings.TrimPrefix(header.Name, "/")
		if _, exists := entries[name]; exists {
			t.Fatalf("duplicate archive entry %s", name)
		}
		data, err := io.ReadAll(reader)
		if err != nil {
			t.Fatal(err)
		}
		entries[name] = data
	}
}

func writePackageEntries(t *testing.T, entries map[string][]byte) string {
	t.Helper()
	filename := filepath.Join(t.TempDir(), "modified.cnb")
	f, err := os.Create(filename)
	if err != nil {
		t.Fatal(err)
	}
	writer := tar.NewWriter(f)
	names := make([]string, 0, len(entries))
	for name := range entries {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		data := entries[name]
		if err := writer.WriteHeader(&tar.Header{Name: name, Mode: 0644, Size: int64(len(data))}); err != nil {
			t.Fatal(err)
		}
		if _, err := writer.Write(data); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	return filename
}

func TestMultiPlatformPackageFile(t *testing.T) {
	t.Parallel()
	targets := []dist.Target{{OS: "linux", Arch: "amd64"}, {OS: "linux", Arch: "arm64"}}
	filename := packageForTargets(t, targets...)
	entries := packageEntries(t, filename)
	var index v1.Index
	if err := json.Unmarshal(entries[v1.ImageIndexFile], &index); err != nil {
		t.Fatal(err)
	}
	if len(index.Manifests) != len(targets) {
		t.Fatalf("expected two manifests in one file, got %d", len(index.Manifests))
	}
	for i, target := range targets {
		if platform := index.Manifests[i].Platform; platform == nil || platform.Architecture != target.Arch {
			t.Fatalf("unexpected platform descriptor: %+v", platform)
		}
		module, dependencies, err := buildpack.BuildpacksFromOCILayoutBlobForTarget(blob.NewBlob(filename), target)
		if err != nil {
			t.Fatal(err)
		}
		if len(dependencies) != 0 || module.Descriptor().Targets()[0].Arch != target.Arch {
			t.Fatalf("loaded the wrong package for %+v", target)
		}
		reader, err := module.Open()
		if err != nil {
			t.Fatal(err)
		}
		contents, err := io.ReadAll(reader)
		closeErr := reader.Close()
		if err != nil || closeErr != nil || len(contents) == 0 {
			t.Fatalf("package layer is not readable: read=%v close=%v", err, closeErr)
		}
	}
	_, _, err := buildpack.BuildpacksFromOCILayoutBlobForTarget(blob.NewBlob(filename), dist.Target{OS: "linux", Arch: "ppc64le"})
	if err == nil || !strings.Contains(err.Error(), "linux/ppc64le") {
		t.Fatalf("expected a descriptive missing-platform error, got %v", err)
	}
	if _, _, err := buildpack.BuildpacksFromOCILayoutBlob(blob.NewBlob(filename)); err != nil {
		t.Fatalf("existing inspection API failed: %v", err)
	}
}

func TestPackageFilePlatformVariantsAndDistributions(t *testing.T) {
	t.Parallel()
	for _, targets := range [][]dist.Target{
		{{OS: "linux", Arch: "arm", ArchVariant: "v6"}, {OS: "linux", Arch: "arm", ArchVariant: "v7"}},
		{{OS: "linux", Arch: "amd64", Distributions: []dist.Distribution{{Name: "ubuntu", Version: "22.04"}}},
			{OS: "linux", Arch: "amd64", Distributions: []dist.Distribution{{Name: "ubuntu", Version: "24.04"}}}},
	} {
		filename := packageForTargets(t, targets...)
		for _, target := range targets {
			module, _, err := buildpack.BuildpacksFromOCILayoutBlobForTarget(blob.NewBlob(filename), target)
			if err != nil {
				t.Fatal(err)
			}
			got, _ := json.Marshal(module.Descriptor().Targets()[0])
			want, _ := json.Marshal(target)
			if string(got) != string(want) {
				t.Fatalf("target selection mismatch: got %s want %s", got, want)
			}
		}
	}
}

func TestPackageFileNestedIndex(t *testing.T) {
	t.Parallel()
	entries := packageEntries(t, packageForTargets(t, dist.Target{OS: "linux", Arch: "amd64"}, dist.Target{OS: "linux", Arch: "arm64"}))
	child := entries[v1.ImageIndexFile]
	childDigest := digest.FromBytes(child)
	entries["blobs/sha256/"+childDigest.Encoded()] = child
	index := v1.Index{}
	index.SchemaVersion = 2
	index.Manifests = []v1.Descriptor{{MediaType: v1.MediaTypeImageIndex, Digest: childDigest, Size: int64(len(child))}}
	var err error
	entries[v1.ImageIndexFile], err = json.Marshal(index)
	if err != nil {
		t.Fatal(err)
	}
	module, _, err := buildpack.BuildpacksFromOCILayoutBlobForTarget(blob.NewBlob(writePackageEntries(t, entries)), dist.Target{OS: "linux", Arch: "arm64"})
	if err != nil {
		t.Fatal(err)
	}
	if got := module.Descriptor().Targets()[0].Arch; got != "arm64" {
		t.Fatalf("selected %s from nested index", got)
	}
}

func TestClosedFilePackageBuilder(t *testing.T) {
	t.Parallel()
	writer, err := buildpack.NewFilePackageBuilder()
	if err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("second Close failed: %v", err)
	}
	if err := writer.Save(filepath.Join(t.TempDir(), "closed.cnb")); err == nil {
		t.Fatal("saved a closed package")
	}
	if err := writer.Add(buildpack.NewBuilder(nil), dist.Target{OS: "linux"}, nil); err == nil {
		t.Fatal("added to a closed package")
	}
}
