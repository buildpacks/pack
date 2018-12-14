package pack_test

import (
	"math/rand"
	"testing"
	"time"

	"github.com/fatih/color"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	"github.com/buildpack/pack"
	h "github.com/buildpack/pack/testhelpers"
)

func TestCache(t *testing.T) {
	color.NoColor = true
	rand.Seed(time.Now().UTC().UnixNano())

	spec.Run(t, "cache", testCache, spec.Parallel(), spec.Report(report.Terminal{}))
}

func testCache(t *testing.T, when spec.G, it spec.S) {
	when("#CacheVolume", func(){
		it("reusing the same cache for the same repo name", func() {
			volume, err := pack.CacheVolume("my/repo")
			h.AssertNil(t, err)
			expected, _ := pack.CacheVolume("my/repo")
			if volume != expected {
				t.Fatalf("The same repo name should result in the same volume")
			}
		})

		it("supplies different volumes for different tags", func() {
			volume, err :=  pack.CacheVolume("my/repo:other-tag")
			h.AssertNil(t, err)
			notExpected, _ := pack.CacheVolume("my/repo")
			if volume == notExpected {
				t.Fatalf("Different image tags should result in different volumes")
			}
		})

		it("supplies different volumes for different registries", func() {
			volume, err :=  pack.CacheVolume("registry.com/my/repo:other-tag")
			h.AssertNil(t, err)
			notExpected, _ := pack.CacheVolume("my/repo")
			if volume == notExpected {
				t.Fatalf("Different image registries should result in different volumes")
			}
		})

		it("resolves implied tag", func() {
			volume, err :=  pack.CacheVolume("my/repo:latest")
			h.AssertNil(t, err)
			expected, _ := pack.CacheVolume("my/repo")
			if volume != expected {
				t.Fatalf("The same repo name should result in the same volume")
			}
		})

		it("resolves implied registry", func() {
			volume, err :=  pack.CacheVolume("index.docker.io/my/repo")
			h.AssertNil(t, err)
			expected, _ := pack.CacheVolume("my/repo")
			if volume != expected {
				t.Fatalf("The same repo name should result in the same volume")
			}
		})
	})
}
