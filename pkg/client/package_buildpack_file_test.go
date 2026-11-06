package client_test

import (
	"archive/tar"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/buildpacks/lifecycle/api"
	"github.com/golang/mock/gomock"
	v1 "github.com/opencontainers/image-spec/specs-go/v1"

	pkgconfig "github.com/buildpacks/pack/buildpackage"
	"github.com/buildpacks/pack/internal/fakes"
	"github.com/buildpacks/pack/pkg/client"
	"github.com/buildpacks/pack/pkg/dist"
	"github.com/buildpacks/pack/pkg/testmocks"
)

func TestMultiPlatformBuildpackProducesOneFile(t *testing.T) {
	t.Parallel()
	for _, failSecondTarget := range []bool{false, true} {
		t.Run(fmt.Sprintf("second-target-failure=%t", failSecondTarget), func(t *testing.T) {
			controller := gomock.NewController(t)
			downloader := testmocks.NewMockBlobDownloader(controller)
			// No daemon calls are expected when packaging files.
			daemon := testmocks.NewMockAPIClient(controller)
			targets := []dist.Target{{OS: "linux", Arch: "amd64"}, {OS: "linux", Arch: "arm64"}}
			source, err := fakes.NewFakeBuildpackBlob(&dist.BuildpackDescriptor{
				WithAPI: api.MustParse("0.10"), WithInfo: dist.ModuleInfo{ID: "example/multi", Version: "1.0.0"}, WithTargets: targets,
			}, 0755)
			if err != nil {
				t.Fatal(err)
			}
			uri := "https://example.com/buildpack.tgz"
			first := downloader.EXPECT().Download(gomock.Any(), uri).Return(source, nil)
			if failSecondTarget {
				gomock.InOrder(first, downloader.EXPECT().Download(gomock.Any(), uri).Return(nil, fmt.Errorf("target download failed")))
			} else {
				first.Times(2)
			}
			subject, err := client.NewClient(client.WithDownloader(downloader), client.WithDockerClient(daemon))
			if err != nil {
				t.Fatal(err)
			}
			directory := t.TempDir()
			filename := filepath.Join(directory, "multi.cnb")
			err = subject.PackageBuildpack(context.Background(), client.PackageBuildpackOptions{
				Name: filename, Format: client.FormatFile, Targets: targets,
				Config: pkgconfig.Config{Buildpack: dist.BuildpackURI{URI: uri}},
			})
			files, globErr := filepath.Glob(filepath.Join(directory, "*.cnb"))
			if globErr != nil {
				t.Fatal(globErr)
			}
			if failSecondTarget {
				if err == nil || len(files) != 0 {
					t.Fatalf("failed packaging produced artifacts: error=%v files=%v", err, files)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if len(files) != 1 || files[0] != filename {
				t.Fatalf("expected one output file %s, got %v", filename, files)
			}
			f, err := os.Open(filename)
			if err != nil {
				t.Fatal(err)
			}
			defer f.Close()
			reader := tar.NewReader(f)
			for {
				header, err := reader.Next()
				if err != nil {
					t.Fatalf("package index not found: %v", err)
				}
				if header.Name != "/index.json" && header.Name != "index.json" {
					continue
				}
				data, err := io.ReadAll(reader)
				if err != nil {
					t.Fatal(err)
				}
				var index v1.Index
				if err := json.Unmarshal(data, &index); err != nil {
					t.Fatal(err)
				}
				if len(index.Manifests) != 2 || index.Manifests[0].Platform.Architecture != "amd64" || index.Manifests[1].Platform.Architecture != "arm64" {
					t.Fatalf("unexpected package index: %+v", index.Manifests)
				}
				break
			}
		})
	}
}
