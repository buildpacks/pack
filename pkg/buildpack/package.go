package buildpack

import (
	"io"
	"strings"
	"sync"

	"github.com/pkg/errors"

	"github.com/buildpacks/pack/internal/style"
	"github.com/buildpacks/pack/pkg/dist"
)

type Package interface {
	Label(name string) (value string, err error)
	GetLayer(diffID string) (io.ReadCloser, error)
}

type syncPkg struct {
	mu  sync.Mutex
	pkg Package
}

func (s *syncPkg) Label(name string) (value string, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.pkg.Label(name)
}

func (s *syncPkg) GetLayer(diffID string) (io.ReadCloser, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.pkg.GetLayer(diffID)
}

func extractBuildpacks(pkg Package) (mainBP BuildModule, depBPs []BuildModule, err error) {
	pkg = &syncPkg{pkg: pkg}
	md := &Metadata{}
	if found, err := dist.GetLabel(pkg, MetadataLabel, md); err != nil {
		return nil, nil, err
	} else if !found {
		return nil, nil, errors.Errorf(
			"could not find label %s",
			style.Symbol(MetadataLabel),
		)
	}

	pkgLayers := dist.ModuleLayers{}
	ok, err := dist.GetLabel(pkg, dist.BuildpackLayersLabel, &pkgLayers)
	if err != nil {
		return nil, nil, err
	}

	if !ok {
		return nil, nil, errors.Errorf(
			"could not find label %s",
			style.Symbol(dist.BuildpackLayersLabel),
		)
	}

	// Example `dist.ModuleLayers{}`:
	//
	//{
	//  "samples/hello-moon": {
	//    "0.0.1": {
	//      "api": "0.2",
	//      "stacks": [
	//        {
	//          "id": "io.buildpacks.samples.stacks.jammy"
	//        },
	//        {
	//          "id": "io.buildpacks.samples.stacks.alpine"
	//        },
	//        {
	//          "id": "io.buildpacks.stacks.jammy"
	//        },
	//        {
	//          "id": "*"
	//        }
	//      ],
	//      "layerDiffID": "sha256:37ab46923c181aa5fb27c9a23479a38aec2679237f35a0ea4115e5ae81a17bba",
	//      "homepage": "https://github.com/buildpacks/samples/tree/main/buildpacks/hello-moon",
	//      "name": "Hello Moon Buildpack"
	//    }
	//  }
	//}

	// We need to know which buildpacks are flattened
	flattenedDiffIDs := flattenedBuildpacksDiffIDs(pkgLayers)

	// Randomly first buildpacks returns all the tar content and subsequent buildpacks return an empty tar
	var seenDiffIDs = make(map[string]struct{})
	for bpID, v := range pkgLayers {
		for bpVersion, bpInfo := range v {
			desc := dist.BuildpackDescriptor{
				WithAPI: bpInfo.API,
				WithInfo: dist.ModuleInfo{
					ID:       bpID,
					Version:  bpVersion,
					Homepage: bpInfo.Homepage,
					Name:     bpInfo.Name,
				},
				WithStacks:  bpInfo.Stacks,
				WithTargets: bpInfo.Targets,
				WithOrder:   bpInfo.Order,
			}

			diffID := bpInfo.LayerDiffID // Allow use in closure

			var openerFunc func() (io.ReadCloser, error)
			if _, ok := seenDiffIDs[diffID]; ok {
				// it is a flattened buildpack, and we want to avoid multiples tar with the same content
				openerFunc = func() (io.ReadCloser, error) {
					return io.NopCloser(strings.NewReader("")), nil
				}
			} else {
				openerFunc = func() (io.ReadCloser, error) {
					rc, err := pkg.GetLayer(diffID)
					if err != nil {
						return nil, errors.Wrapf(err,
							"extracting buildpack %s layer (diffID %s)",
							style.Symbol(desc.Info().FullName()),
							style.Symbol(diffID),
						)
					}
					return rc, nil
				}
				seenDiffIDs[diffID] = member
			}

			b := &openerBlob{
				opener: openerFunc,
			}

			var blobOpts []BlobOption
			if _, ok := flattenedDiffIDs[diffID]; ok {
				blobOpts = append(blobOpts, Flattened())
			}
			if desc.Info().Match(md.ModuleInfo) { // This is the order buildpack of the package
				mainBP = FromBlob(&desc, b, blobOpts...)
			} else {
				depBPs = append(depBPs, FromBlob(&desc, b, blobOpts...))
			}
		}
	}

	return mainBP, depBPs, nil
}

func extractExtensions(pkg Package) (mainExt BuildModule, err error) {
	pkg = &syncPkg{pkg: pkg}
	md := &Metadata{}
	if found, err := dist.GetLabel(pkg, MetadataLabel, md); err != nil {
		return nil, err
	} else if !found {
		return nil, errors.Errorf(
			"could not find label %s",
			style.Symbol(MetadataLabel),
		)
	}

	pkgLayers := dist.ModuleLayers{}
	ok, err := dist.GetLabel(pkg, dist.ExtensionLayersLabel, &pkgLayers)
	if err != nil {
		return nil, err
	}

	if !ok {
		return nil, errors.Errorf(
			"could not find label %s",
			style.Symbol(dist.ExtensionLayersLabel),
		)
	}
	for extID, v := range pkgLayers {
		for extVersion, extInfo := range v {
			desc := dist.ExtensionDescriptor{
				WithAPI: extInfo.API,
				WithInfo: dist.ModuleInfo{
					ID:       extID,
					Version:  extVersion,
					Homepage: extInfo.Homepage,
					Name:     extInfo.Name,
				},
			}

			diffID := extInfo.LayerDiffID // Allow use in closure
			b := &openerBlob{
				opener: func() (io.ReadCloser, error) {
					rc, err := pkg.GetLayer(diffID)
					if err != nil {
						return nil, errors.Wrapf(err,
							"extracting extension %s layer (diffID %s)",
							style.Symbol(desc.Info().FullName()),
							style.Symbol(diffID),
						)
					}
					return rc, nil
				},
			}

			mainExt = FromBlob(&desc, b)
		}
	}
	return mainExt, nil
}

type void struct{}

var member void

func flattenedBuildpacksDiffIDs(layers dist.ModuleLayers) map[string]struct{} {
	var seenDiffID = make(map[string]struct{})
	var flattenedDiffIDs = make(map[string]struct{})

	for _, values := range layers {
		for _, bpInfo := range values {
			if _, ok := seenDiffID[bpInfo.LayerDiffID]; ok {
				flattenedDiffIDs[bpInfo.LayerDiffID] = member
			} else {
				seenDiffID[bpInfo.LayerDiffID] = member
			}
		}
	}

	return flattenedDiffIDs
}

type openerBlob struct {
	opener func() (io.ReadCloser, error)
}

func (b *openerBlob) Open() (io.ReadCloser, error) {
	return b.opener()
}
