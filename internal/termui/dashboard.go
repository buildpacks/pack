package termui

import (
	"fmt"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"

	"github.com/buildpacks/pack/pkg/dist"
)

type Dashboard struct {
	app         app
	appTree     *tview.TreeView
	builderTree *tview.TreeView
	planList    *tview.List
	logsView    *tview.TextView
	logs        string
}

func NewDashboard(app app, appName string, bldr buildr, runImageName string, buildpackInfo []dist.BuildpackInfo) *Dashboard {
	appTree, builderTree := initTrees(appName, bldr, runImageName)

	planList, logsView := initDashboard(buildpackInfo)

	d := &Dashboard{
		app:         app,
		appTree:     appTree,
		builderTree: builderTree,
		planList:    planList,
		logsView:    logsView,
	}

	imagesView := tview.NewFlex().
		SetDirection(tview.FlexRow).
		AddItem(appTree, 0, 1, false).
		AddItem(builderTree, 0, 1, true)

	imagesView.
		SetBorder(true).
		SetTitleAlign(tview.AlignLeft).
		SetTitle("| [::b]images[::-] |").
		SetBackgroundColor(backgroundColor)

	leftPane := tview.NewFlex().
		SetDirection(tview.FlexRow).
		AddItem(imagesView, 11, 0, false).
		AddItem(planList, 0, 1, true)

	screen := tview.NewFlex().
		SetDirection(tview.FlexColumn).
		AddItem(leftPane, 0, 1, true).
		AddItem(logsView, 0, 1, false)

	d.app.SetRoot(screen, true)
	return d
}

func (d *Dashboard) Handle(txt string) {
	d.app.QueueUpdateDraw(func() {
		d.logs = d.logs + txt + "\n"
		d.logsView.SetText(tview.TranslateANSI(d.logs))
	})
}

func (d *Dashboard) Stop() {
	// no-op
}

func initDashboard(buildpackInfos []dist.BuildpackInfo) (*tview.List, *tview.TextView) {
	planList := tview.NewList()
	planList.SetMainTextColor(tcell.ColorMediumTurquoise).
		SetSelectedTextColor(tcell.ColorMediumTurquoise).
		SetSelectedBackgroundColor(tcell.ColorDarkSlateGray).
		SetSecondaryTextColor(tcell.ColorDimGray).
		SetBorder(true).
		SetBorderPadding(1, 1, 1, 1).
		SetTitle("| [::b]plan[::-] |").
		SetTitleAlign(tview.AlignLeft).
		SetBackgroundColor(backgroundColor)

	for _, buildpackInfo := range buildpackInfos {
		planList.AddItem(
			buildpackInfo.FullName(),
			info(buildpackInfo),
			'✔',
			func() {},
		)
	}

	logsView := tview.NewTextView()
	logsView.SetDynamicColors(true).
		SetTextAlign(tview.AlignLeft).
		SetBorderPadding(1, 1, 3, 1).
		SetTitleAlign(tview.AlignLeft).
		SetBackgroundColor(backgroundColor)

	return planList, logsView
}

func initTrees(appName string, bldr buildr, runImageName string) (*tview.TreeView, *tview.TreeView) {
	var (
		appImage     = tview.NewTreeNode(fmt.Sprintf("app: [white::b]%s", appName)).SetColor(tcell.ColorDimGray)
		appRunImage  = tview.NewTreeNode(fmt.Sprintf(" run: [white::b]%s", runImageName)).SetColor(tcell.ColorDimGray)
		builderImage = tview.NewTreeNode(fmt.Sprintf("builder: [white::b]%s", bldr.BaseImageName())).SetColor(tcell.ColorDimGray)
		lifecycle    = tview.NewTreeNode(fmt.Sprintf(" lifecycle: [white::b]%s", bldr.LifecycleDescriptor().Info.Version.String())).SetColor(tcell.ColorDimGray)
		runImage     = tview.NewTreeNode(fmt.Sprintf(" run: [white::b]%s", bldr.Stack().RunImage.Image)).SetColor(tcell.ColorDimGray)
		buildpacks   = tview.NewTreeNode(" [mediumturquoise::b]buildpacks")
	)

	appImage.AddChild(appRunImage)
	builderImage.AddChild(lifecycle)
	builderImage.AddChild(runImage)
	builderImage.AddChild(buildpacks)

	appTree := tview.NewTreeView()
	appTree.
		SetRoot(appImage).
		SetGraphics(true).
		SetGraphicsColor(tcell.ColorMediumTurquoise).
		SetTitleAlign(tview.AlignLeft).
		SetBorderPadding(1, 0, 4, 0).
		SetBackgroundColor(backgroundColor)

	builderTree := tview.NewTreeView()
	builderTree.
		SetRoot(builderImage).
		SetGraphics(true).
		SetGraphicsColor(tcell.ColorMediumTurquoise).
		SetTitleAlign(tview.AlignLeft).
		SetBorderPadding(0, 0, 4, 0).
		SetBackgroundColor(backgroundColor)

	return appTree, builderTree
}

func info(buildpackInfo dist.BuildpackInfo) string {
	if buildpackInfo.Description != "" {
		return buildpackInfo.Description
	}

	return buildpackInfo.Homepage
}
