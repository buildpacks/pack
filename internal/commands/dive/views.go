package dive

import (
	"github.com/jroimartin/gocui"

	"github.com/buildpacks/pack"
)

type Views struct {
	Tree    *FileTree
	Layer   *Layer
	Status  *Status
	Details ImageDetails
}

func (views *Views) All() []Renderer {
	return []Renderer{
		views.Layer,
		views.Tree,
		views.Status,
		views.Details,
	}
}

func NewViews(g *gocui.Gui, diveResult *pack.DiveResult) (*Views, error) {
	Layer, err := NewLayerView(g, diveResult.CNBImage.Layers)
	if err != nil {
		return nil, err
	}

	treeStack := diveResult.CNBImage.Trees[0]
	// TODO the interfaces on all of these should really be changed....
	Tree, err := NewFileTreeView(g, treeStack, diveResult.CNBImage.Trees, diveResult.TreeStack)
	if err != nil {
		return nil, err
	}

	Status := newStatusView(g)

	// set the layer view as the first selected view
	Status.SetCurrentView(Layer)

	//Filter := newFilterView(g)

	// TODO add switches here so that this is in an if condition

	// this call should be factored out and only used once....
	//client, err := pack.NewClient()
	//if err != nil {
	//	return nil, fmt.Errorf("unable to create pack client: %s", err)
	//}
	//
	//imgInfo, err := client.InspectImage("java-test", true)
	//if err != nil {
	//	return nil, fmt.Errorf("unable to retrieve %s image info: %s", "my-test", err)
	//}

	Details := NewCNBDetailsView(g, diveResult)
	//
	//Debug := newDebugView(g)

	return &Views{
		Tree:   Tree,
		Layer:  Layer,
		Status: Status,
		//Filter:  Filter,
		Details: Details,
		//Debug:   Debug,
	}, nil
}
