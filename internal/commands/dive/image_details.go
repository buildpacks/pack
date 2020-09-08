package dive

import (
	"github.com/jroimartin/gocui"
	"github.com/wagoodman/dive/dive/image"
)

type ImageDetails interface {
	OnLayoutChange() error
	Name() string
	Setup(*gocui.View, *gocui.View) error
	SetCurrentLayer(layer *image.Layer)
	Renderer
}

type Renderer interface {
	Update() error
	Render() error
	IsVisible() bool
}
