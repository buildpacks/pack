package pack

import (
	"context"
	"fmt"

	"github.com/buildpacks/lifecycle"
	"github.com/pkg/errors"
	"github.com/wagoodman/dive/dive/filetree"
	diveimage "github.com/wagoodman/dive/dive/image"
	divedocker "github.com/wagoodman/dive/dive/image/docker"

	"github.com/buildpacks/pack/config"
	"github.com/buildpacks/pack/internal/dist"
	"github.com/buildpacks/pack/internal/image"
)

// we need to make a couple data structures to allow for fast lookup
//sha256 -> BuildpackInfo?
type LayerLookup struct {
	lifecycle.LayersMetadata
	LayerToBuildpack map[string]lifecycle.BuildpackLayersMetadata
}

// good idea to wrap this in a config object we can extend later.
type DiveImage struct {
	Name string
	*diveimage.Image
	CNBImage *diveimage.Image
}

func NewLayerLookup(label lifecycle.LayersMetadata) LayerLookup {
	var result LayerLookup
	result.LayerToBuildpack = make(map[string]lifecycle.BuildpackLayersMetadata)
	result.LayersMetadata = label
	for _, buildpack := range result.Buildpacks {
		for _, layer := range buildpack.Layers {
			result.LayerToBuildpack[layer.SHA] = buildpack
		}
	}
	return result
}

type DiveResult struct {
	LayerLookupInfo LayerLookup
	Image           DiveImage
	TreeStack       filetree.Comparer
	BuildMetadata   lifecycle.BuildMetadata
	CNBImage        *diveimage.Image
}

func (c *Client) Dive(name string, daemon bool) (*DiveResult, error) {
	// TODO: fetch needs to download a local image for inspection,
	// TODO: this should do all preprocessing, build trees from archive etc.
	// this operation cannot be preformed with remote images.

	// TODO: use pull policy to infer daemon usage
	img, err := c.imageFetcher.Fetch(context.Background(), name, daemon, config.PullNever)
	if err != nil {
		if errors.Cause(err) == image.ErrNotFound {
			return nil, nil
		}
		return nil, err
	}

	var layersMd lifecycle.LayersMetadata
	if _, err := dist.GetLabel(img, lifecycle.LayerMetadataLabel, &layersMd); err != nil {
		return nil, err
	}

	var buildMD lifecycle.BuildMetadata
	if _, err := dist.GetLabel(img, lifecycle.BuildMetadataLabel, &buildMD); err != nil {
		return nil, err
	}

	readCloser, err := c.docker.ImageSave(context.Background(), []string{name})
	if err != nil {
		panic(err)
		return nil, err
	}

	diveImageArchive, err := divedocker.NewImageArchive(readCloser)
	if err != nil {
		panic(err)
		return nil, err
	}

	diveImage, err := diveImageArchive.ToImage()
	if err != nil {
		return nil, err
	}

	cnbImage, err := analyzeCNB(diveImage, layersMd.RunImage.TopLayer)
	if err != nil {
		return nil, err
	}

	treeStack := filetree.NewComparer(cnbImage.Trees)
	buildErrors := treeStack.BuildCache()
	if len(buildErrors) != 0 {
		panic(buildErrors)
	}

	var result DiveResult
	result.Image = DiveImage{Name: name, Image: diveImage}
	result.TreeStack = treeStack
	result.BuildMetadata = buildMD
	result.CNBImage = cnbImage

	result.LayerLookupInfo = NewLayerLookup(layersMd)

	return &result, nil
}

func analyzeCNB(diveImg *diveimage.Image, topOfStackSha string) (*diveimage.Image, error) {
	// first lets get the cnb info that we need

	result := diveimage.Image{}
	newLayers := []*diveimage.Layer{}
	newRefTree := []*filetree.FileTree{}

	if len(diveImg.Layers) != len(diveImg.Trees) {
		return nil, fmt.Errorf("mismatched lengths %s vs %s", len(diveImg.Layers), len(diveImg.Trees))
	}

	var curLayer *diveimage.Layer = nil
	var curRefTree *filetree.FileTree = nil
	var isStack bool = true
	for layerIdx,layer := range diveImg.Layers {
		rTree := diveImg.Trees[layerIdx]
		if curLayer == nil {
			curLayer = layer
			curRefTree = rTree
			continue
		}
		if isStack { // in stack still
			curLayer.Size += layer.Size
			_, err := curRefTree.Stack(rTree)
			if err != nil {
				return nil, fmt.Errorf("error to stacking trees")
			}
		}
		if layer.Digest == topOfStackSha { // end of stack
			newLayers = append(newLayers, curLayer)
			newRefTree = append(newRefTree, curRefTree)
			isStack = false
		}
		if !isStack {
			newLayers = append(newLayers, layer)
			newRefTree = append(newRefTree, rTree)
		}
	}
	result.Trees = newRefTree
	result.Layers = newLayers

	return &result, nil



}