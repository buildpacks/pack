package testhelpers

import (
	"bytes"
	"strings"

	"github.com/google/go-containerregistry/pkg/name"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/types"

	"github.com/buildpacks/pack/pkg/dist"
)

func FakeIndexManifestBuilderFn(targets []dist.Target) func(name.Reference) (*v1.IndexManifest, error) {
	var manifests = make([]v1.Descriptor, 0)
	for _, t := range targets {
		if err := t.Range(func(target dist.Target, distroName, distroVersion string) error {
			targetStr := strings.Join([]string{
				t.OS,
				t.Arch,
				t.ArchVariant,
			}, "")

			hash, size, err := v1.SHA256(bytes.NewBufferString(strings.Join([]string{targetStr, distroName, distroVersion}, "")))
			manifests = append(manifests, v1.Descriptor{
				MediaType:   types.OCIManifestSchema1,
				Size:        size,
				Digest:      hash,
				URLs:        t.URLs(),
				Annotations: t.Specs.Annotations,
				Platform: &v1.Platform{
					OS:           t.OS,
					Architecture: t.Arch,
					Variant:      t.ArchVariant,
					OSVersion:    distroVersion,
					OSFeatures:   t.Specs.OSFeatures,
					Features:     t.Specs.Features,
				},
			})
			return err
		}); err != nil {
			return func(name.Reference) (*v1.IndexManifest, error) {
				return nil, err
			}
		}
	}

	return func(ref name.Reference) (*v1.IndexManifest, error) {
		return &v1.IndexManifest{
			MediaType:     types.OCIImageIndex,
			SchemaVersion: 1,
			Manifests:     manifests,
			Annotations:   map[string]string{"some-key": "some-version"},
		}, nil
	}
}
