package buildpackage

import (
	"io"

	"github.com/pkg/errors"

	"github.com/buildpacks/pack/internal/dist"
	"github.com/buildpacks/pack/internal/style"
)

type Package interface {
	Label(name string) (value string, err error)
	GetLayer(diffID string) (io.ReadCloser, error)
}

func ExtractBuildpacks(pkgImage Package) (mainBP dist.Buildpack, depBPs []dist.Buildpack, err error) {
	md := &Metadata{}
	if found, err := dist.GetLabel(pkgImage, MetadataLabel, md); err != nil {
		return nil, nil, err
	} else if !found {
		return nil, nil, errors.Errorf(
			"could not find label %s",
			style.Symbol(MetadataLabel),
		)
	}

	bpLayers := dist.BuildpackLayers{}
	ok, err := dist.GetLabel(pkgImage, dist.BuildpackLayersLabel, &bpLayers)
	if err != nil {
		return nil, nil, err
	}

	if !ok {
		return nil, nil, errors.Errorf(
			"could not find label %s",
			style.Symbol(dist.BuildpackLayersLabel),
		)
	}

	for bpID, v := range bpLayers {
		for bpVersion, bpInfo := range v {
			desc := dist.BuildpackDescriptor{
				API: bpInfo.API,
				Info: dist.BuildpackInfo{
					ID:      bpID,
					Version: bpVersion,
				},
				Stacks: bpInfo.Stacks,
				Order:  bpInfo.Order,
			}

			diffID := bpInfo.LayerDiffID // Allow use in closure
			b := &openerBlob{
				opener: func() (io.ReadCloser, error) {
					rc, err := pkgImage.GetLayer(diffID)
					if err != nil {
						return nil, errors.Wrapf(err,
							"extracting buildpack %s layer (diffID %s)",
							style.Symbol(desc.Info.FullName()),
							style.Symbol(diffID),
						)
					}
					return rc, nil
				},
			}

			if desc.Info == md.BuildpackInfo {
				mainBP = dist.BuildpackFromBlob(desc, b)
			} else {
				depBPs = append(depBPs, dist.BuildpackFromBlob(desc, b))
			}
		}
	}

	return mainBP, depBPs, nil
}

type openerBlob struct {
	opener func() (io.ReadCloser, error)
}

func (b *openerBlob) Open() (io.ReadCloser, error) {
	return b.opener()
}
