package dive

import (
	"fmt"

	"github.com/rs/zerolog/log"

	"github.com/jroimartin/gocui"
	"github.com/sirupsen/logrus"
	"github.com/wagoodman/dive/dive/image"
	"github.com/wagoodman/dive/runtime/ui/format"
	"github.com/wagoodman/dive/runtime/ui/key"
	"github.com/wagoodman/dive/runtime/ui/viewmodel"
)

func (v *Layer) CurrentLayer() *image.Layer {
	return v.vm.Layers[v.vm.LayerIndex]
}

type LayerChangeListener func(viewmodel.LayerSelection) error

///
/// LAYERS
///
///

type Layer struct {
	name                  string
	gui                   *gocui.Gui
	view                  *gocui.View
	header                *gocui.View
	vm                    *LayerSetState
	constrainedRealEstate bool

	listeners []LayerChangeListener

	helpKeys []*key.Binding
}

func NewLayerView(gui *gocui.Gui, layers []*image.Layer) (controller *Layer, err error) {
	controller = new(Layer)

	controller.listeners = make([]LayerChangeListener, 0)

	// populate main fields
	controller.name = "layer"
	controller.gui = gui

	var compareMode viewmodel.LayerCompareMode = viewmodel.CompareSingleLayer

	//switch mode := viper.GetBool("layer.show-aggregated-changes"); mode {
	//case true:
	//	compareMode = viewmodel.CompareAllLayers
	//case false:
	//	compareMode = viewmodel.CompareSingleLayer
	//default:
	//	return nil, fmt.Errorf("unknown layer.show-aggregated-changes value: %v", mode)
	//}

	controller.vm = NewLayerSetState(layers, compareMode)

	return controller, err
}

// KeyHelp indicates all the possible actions a user can take while the current pane is selected.
func (v *Layer) KeyHelp() string {
	var help string
	for _, binding := range v.helpKeys {
		help += binding.RenderKeyHelp()
	}
	return help
}

// Update refreshes the state objects for future rendering (currently does nothing).
func (v *Layer) Update() error {
	return nil
}

// Render flushes the state objects to the screen. The layers pane reports:
// 1. the layers of the image + metadata
// 2. the current selected image
func (v *Layer) Render() error {
	logrus.Tracef("view.Render() %s", v.Name())

	// indicate when selected
	title := "Layers"
	isSelected := v.gui.CurrentView() == v.view

	v.gui.Update(func(g *gocui.Gui) error {
		var err error
		// update header
		v.header.Clear()
		width, _ := g.Size()
		if v.constrainedRealEstate {
			headerStr := format.RenderNoHeader(width, isSelected)
			headerStr += "\nLayer"
			_, err := fmt.Fprintln(v.header, headerStr)
			if err != nil {
				return err
			}
		} else {
			headerStr := format.RenderHeader(title, width, isSelected)
			headerStr += fmt.Sprintf("Cmp"+image.LayerFormat, "Size", "Command")
			_, err := fmt.Fprintln(v.header, headerStr)
			if err != nil {
				return err
			}
		}

		// update contents
		v.view.Clear()
		for idx, layer := range v.vm.Layers {

			var layerStr string
			if v.constrainedRealEstate {
				layerStr = fmt.Sprintf("%-4d", layer.Index)
			} else {
				layerStr = layer.String()
			}

			compareBar := v.renderCompareBar(idx)

			if idx == v.vm.LayerIndex {
				_, err = fmt.Fprintln(v.view, compareBar+" "+format.Selected(layerStr))
			} else {
				_, err = fmt.Fprintln(v.view, compareBar+" "+layerStr)
			}

			if err != nil {
				log.Print("unable to write to buffer: ", err)
				return err
			}

		}
		return nil
	})
	return nil
}

// IsVisible indicates if the layer view pane is currently initialized.
func (v *Layer) IsVisible() bool {
	return v != nil
}

func (v *Layer) Name() string {
	return v.name
}

// renderCompareBar returns the formatted string for the given layer.
func (v *Layer) renderCompareBar(layerIdx int) string {
	bottomTreeStart, bottomTreeStop, topTreeStart, topTreeStop := v.vm.GetCompareIndexes()
	result := "  "

	if layerIdx >= bottomTreeStart && layerIdx <= bottomTreeStop {
		result = format.CompareBottom("  ")
	}
	if layerIdx >= topTreeStart && layerIdx <= topTreeStop {
		result = format.CompareTop("  ")
	}

	return result
}

// OnLayoutChange is called whenever the screen dimensions are changed
func (v *Layer) OnLayoutChange() error {
	err := v.Update()
	if err != nil {
		return err
	}
	return v.Render()
}

func (v *Layer) AddLayerChangeListener(listener ...LayerChangeListener) {
	v.listeners = append(v.listeners, listener...)
}

func (v *Layer) LayerCount() int {
	return len(v.vm.Layers)
}

func (v *Layer) Setup(view *gocui.View, header *gocui.View) error {
	logrus.Tracef("view.Setup() %s", v.Name())

	// set controller options
	v.view = view
	v.view.Editable = false
	v.view.Wrap = false
	v.view.Frame = false

	v.header = header
	v.header.Editable = false
	v.header.Wrap = false
	v.header.Frame = false

	var infos = []key.BindingInfo{
		//{
		//	ConfigKeys: []string{"keybinding.compare-layer"},
		//	OnAction:   func() error { return v.setCompareMode(viewmodel.CompareSingleLayer) },
		//	IsSelected: func() bool { return v.vm.CompareMode == viewmodel.CompareSingleLayer },
		//	Display:    "Show layer changes",
		//},
		//{
		//	ConfigKeys: []string{"keybinding.compare-all"},
		//	OnAction:   func() error { return v.setCompareMode(viewmodel.CompareAllLayers) },
		//	IsSelected: func() bool { return v.vm.CompareMode == viewmodel.CompareAllLayers },
		//	Display:    "Show aggregated changes",
		//},
		{
			Key:      gocui.KeyArrowDown,
			Modifier: gocui.ModNone,
			OnAction: v.CursorDown,
		},
		{
			Key:      gocui.KeyArrowUp,
			Modifier: gocui.ModNone,
			OnAction: v.CursorUp,
		},
		{
			Key:      gocui.KeyArrowLeft,
			Modifier: gocui.ModNone,
			OnAction: v.CursorUp,
		},
		{
			Key:      gocui.KeyArrowRight,
			Modifier: gocui.ModNone,
			OnAction: v.CursorDown,
		},

		//{
		//	ConfigKeys: []string{"keybinding.page-up"},
		//	OnAction:   v.PageUp,
		//},
		//{
		//	ConfigKeys: []string{"keybinding.page-down"},
		//	OnAction:   v.PageDown,
		//},
	}

	helpKeys, err := key.GenerateBindings(v.gui, v.name, infos)
	if err != nil {
		return err
	}
	v.helpKeys = helpKeys

	return v.Render()
}

func (v *Layer) ConstrainLayout() {
	if !v.constrainedRealEstate {
		log.Printf("constraining layer layout")
		v.constrainedRealEstate = true
	}
}

func (v *Layer) ExpandLayout() {
	if v.constrainedRealEstate {
		log.Printf("expanding layer layout")
		v.constrainedRealEstate = false
	}
}

// CursorDown moves the cursor down in the layer pane (selecting a higher layer).
func (v *Layer) CursorDown() error {
	if v.vm.LayerIndex < len(v.vm.Layers) {
		err := CursorDown(v.gui, v.view)
		if err == nil {
			return v.SetCursor(v.vm.LayerIndex + 1)
		}
	}
	return nil
}

// CursorUp moves the cursor up in the layer pane (selecting a lower layer).
func (v *Layer) CursorUp() error {
	if v.vm.LayerIndex > 0 {
		err := CursorUp(v.gui, v.view)
		if err == nil {
			return v.SetCursor(v.vm.LayerIndex - 1)
		}
	}
	return nil
}

// SetCursor resets the cursor and orients the file tree view based on the given layer index.
func (v *Layer) SetCursor(layer int) error {
	v.vm.LayerIndex = layer
	err := v.notifyLayerChangeListeners()
	if err != nil {
		return err
	}

	return v.Render()
}

func (v *Layer) notifyLayerChangeListeners() error {
	bottomTreeStart, bottomTreeStop, topTreeStart, topTreeStop := v.vm.GetCompareIndexes()
	selection := viewmodel.LayerSelection{
		Layer:           v.CurrentLayer(),
		BottomTreeStart: bottomTreeStart,
		BottomTreeStop:  bottomTreeStop,
		TopTreeStart:    topTreeStart,
		TopTreeStop:     topTreeStop,
	}
	for _, listener := range v.listeners {
		err := listener(selection)
		if err != nil {
			logrus.Errorf("notifyLayerChangeListeners error: %+v", err)
			return err
		}
	}
	return nil
}
