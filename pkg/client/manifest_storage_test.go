package client

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	h "github.com/buildpacks/pack/testhelpers"
)

func TestManifestStorage(t *testing.T) {
	for _, runtimeEnv := range []string{"set", "empty", "unset"} {
		t.Run(runtimeEnv, func(t *testing.T) {
			packHome := t.TempDir()
			runtimeDir := t.TempDir()
			t.Setenv("PACK_HOME", packHome)
			t.Setenv("XDG_RUNTIME_DIR", runtimeDir)
			root := filepath.Join(runtimeDir, "pack", "manifests")
			if runtimeEnv != "set" {
				root = filepath.Join(packHome, "manifests")
				t.Setenv("XDG_RUNTIME_DIR", "")
				if runtimeEnv == "unset" {
					h.AssertNil(t, os.Unsetenv("XDG_RUNTIME_DIR"))
				}
			}

			// A same-named runtime directory belongs to another program.
			otherDir := filepath.Join(runtimeDir, "podman")
			h.AssertNil(t, os.MkdirAll(otherDir, 0700))
			sentinel := filepath.Join(otherDir, "podman.sock")
			h.AssertNil(t, os.WriteFile(sentinel, []byte("preserve me"), 0600))

			subject, err := NewClient(WithExperimental(true))
			h.AssertNil(t, err)
			h.AssertNil(t, subject.CreateManifest(context.Background(), CreateManifestOptions{IndexRepoName: "podman"}))
			manifest := h.ReadIndexManifest(t, filepath.Join(root, "podman"))
			h.AssertEq(t, len(manifest.Manifests), 0)
			if !subject.indexFactory.Exists("podman") {
				t.Fatal("saved index cannot be found in its storage directory")
			}
			_, err = subject.indexFactory.LoadIndex("podman")
			h.AssertNil(t, err)
			if err := subject.CreateManifest(context.Background(), CreateManifestOptions{IndexRepoName: "podman"}); err == nil {
				t.Fatal("creating an existing index should fail")
			}
			data, err := os.ReadFile(sentinel)
			h.AssertNil(t, err)
			h.AssertEq(t, string(data), "preserve me")
		})
	}
}
