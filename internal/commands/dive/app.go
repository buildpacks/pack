package dive

import (
	"github.com/jroimartin/gocui"
	"github.com/sirupsen/logrus"
	"github.com/wagoodman/dive/runtime/ui/key"
	"github.com/wagoodman/dive/runtime/ui/layout"
	"time"

	"github.com/buildpacks/pack"
)

type App struct {
	gui         *gocui.Gui
	controllers *Controller
	layout      *layout.Manager
}

func (a *App) Run() error {
	go func() {
		time.Sleep(1 * time.Minute)
		a.Quit()
	}()

	if err := a.gui.MainLoop(); err != nil && err != gocui.ErrQuit {
		logrus.Error("main loop error: ", err)
		return err
	}
	return nil
}

func (a *App) Quit() error {

	// profileObj.Stop()
	// onExit()

	return gocui.ErrQuit
}

type AppOptions struct {
	DiveResult *pack.DiveResult
	GUI        *gocui.Gui
}

func NewApp(appOptions AppOptions) (*App, error) {
	var err error
	once.Do(func() {
		var controller *Controller
		//var globalHelpKeys []*key.Binding

		controller, err = NewController(appOptions.GUI, appOptions.DiveResult)
		if err != nil {
			return
		}

		// note: order matters when adding elements to the layout
		lm := layout.NewManager()
		lm.Add(NewLayerDetailsCompoundLayout(controller.views.Layer, controller.views.Details), layout.LocationColumn)
		lm.Add(controller.views.Tree, layout.LocationColumn)

		// todo: access this more programmatically

		appOptions.GUI.Cursor = false
		//g.Mouse = true
		appOptions.GUI.SetManagerFunc(lm.Layout)

		// var profileObj = profile.Start(profile.CPUProfile, profile.ProfilePath("."), profile.NoShutdownHook)
		//
		// onExit = func() {
		// 	profileObj.Stop()
		// }

		appSingleton = &App{
			gui:         appOptions.GUI,
			controllers: controller,
			layout:      lm,
		}

		var infos = []key.BindingInfo{
			{
				ConfigKeys: []string{"keybinding.quit"},
				OnAction:   appSingleton.Quit,
				Display:    "Quit",
			},

			//{
			//	ConfigKeys: []string{"keybinding.toggle-view"},
			//	OnAction:   controller.ToggleView,
			//	Display:    "Switch view",
			//},
			//{
			//	ConfigKeys: []string{"keybinding.filter-files"},
			//	OnAction:   controller.ToggleFilterView,
			//	IsSelected: controller.views.Filter.IsVisible,
			//	Display:    "Filter",
			//},
		}

		globalHelpKeys, err := key.GenerateBindings(appOptions.GUI, "", infos)
		if err != nil {
			logrus.Error(globalHelpKeys)
			return
		}

		//controller.views.Status.AddHelpKeys(globalHelpKeys...)

		// perform the first update and render now that all resources have been loaded
		err = controller.UpdateAndRender()
		if err != nil {
			return
		}

	})

	return appSingleton, err
}
