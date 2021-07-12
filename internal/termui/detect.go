package termui

import (
	"time"

	"github.com/rivo/tview"
)

type Detect struct {
	app      app
	textView *tview.TextView
	doneChan chan bool
}

func NewDetect(app app) *Detect {
	d := &Detect{
		app:      app,
		textView: detectStatusTV(app),
		doneChan: make(chan bool, 1),
	}

	go d.start()
	grid := centered(d.textView)
	d.app.SetRoot(grid, true)
	return d
}

func (d *Detect) Stop() {
	d.doneChan <- true
}

func (d *Detect) start() {
	var (
		i        = 0
		ticker   = time.NewTicker(250 * time.Millisecond)
		doneText = "⌛️ Detected!"
		texts    = []string{
			"⏳️ Detecting",
			"⏳️ Detecting.",
			"⏳️ Detecting..",
			"⏳️ Detecting...",
		}
	)

	for {
		select {
		case <-ticker.C:
			d.textView.SetText(texts[i])

			i++
			if i == len(texts) {
				i = 0
			}
		case <-d.doneChan:
			ticker.Stop()
			d.textView.SetText(doneText)
			return
		}
	}
}

func detectStatusTV(app app) *tview.TextView {
	tv := tview.NewTextView()
	tv.SetBackgroundColor(backgroundColor)
	tv.SetChangedFunc(func() { app.Draw() })
	return tv
}

func centered(p tview.Primitive) tview.Primitive {
	return tview.NewGrid().
		SetColumns(0, 20, 0).
		SetRows(0, 1, 0).
		AddItem(tview.NewBox().SetBackgroundColor(backgroundColor), 0, 0, 1, 1, 0, 0, true).
		AddItem(tview.NewBox().SetBackgroundColor(backgroundColor), 0, 1, 1, 1, 0, 0, true).
		AddItem(tview.NewBox().SetBackgroundColor(backgroundColor), 0, 2, 1, 1, 0, 0, true).
		AddItem(tview.NewBox().SetBackgroundColor(backgroundColor), 1, 0, 1, 1, 0, 0, true).
		AddItem(p, 1, 1, 1, 1, 0, 0, true).
		AddItem(tview.NewBox().SetBackgroundColor(backgroundColor), 1, 2, 1, 1, 0, 0, true).
		AddItem(tview.NewBox().SetBackgroundColor(backgroundColor), 2, 0, 1, 1, 0, 0, true).
		AddItem(tview.NewBox().SetBackgroundColor(backgroundColor), 2, 1, 1, 1, 0, 0, true).
		AddItem(tview.NewBox().SetBackgroundColor(backgroundColor), 2, 2, 1, 1, 0, 0, true)
}
