package fakes

import (
	"github.com/rivo/tview"
)

type App struct {
	SetRootCallCount int
	DrawCallCount    int
	RunCallCount     int
}

func NewApp() *App {
	return &App{}
}

func (a *App) SetRoot(root tview.Primitive, fullscreen bool) *tview.Application {
	a.SetRootCallCount++
	return nil
}

func (a *App) Draw() *tview.Application {
	a.DrawCallCount++
	return nil
}

func (a *App) Run() error {
	a.RunCallCount++
	return nil
}
