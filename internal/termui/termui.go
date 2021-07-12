package termui

import (
	"bufio"
	"io"
	"io/ioutil"
	"strings"

	dcontainer "github.com/docker/docker/api/types/container"
	"github.com/docker/docker/pkg/stdcopy"
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"

	"github.com/buildpacks/pack/internal/container"
)

var (
	backgroundColor = tcell.NewRGBColor(5, 30, 40)
)

type app interface {
	SetRoot(root tview.Primitive, fullscreen bool) *tview.Application
	Draw() *tview.Application
	Run() error
}

type page interface {
	Stop()
}

type Termui struct {
	app         app
	currentPage page
}

func NewTermui() *Termui {
	return &Termui{
		app: tview.NewApplication(),
	}
}

// Run starts the terminal UI process in the foreground
// and the passed in function in the background
func (s *Termui) Run(funk func()) error {
	go funk()

	s.currentPage = NewDetect(s.app)
	return s.app.Run()
}

func (s *Termui) Handler() container.Handler {
	return func(bodyChan <-chan dcontainer.ContainerWaitOKBody, errChan <-chan error, reader io.Reader) error {
		var (
			copyErr = make(chan error)
			r, w    = io.Pipe()
			scanner = bufio.NewScanner(r)
		)

		go func() {
			defer w.Close()

			_, err := stdcopy.StdCopy(w, ioutil.Discard, reader)
			if err != nil {
				copyErr <- err
			}
		}()

		for {
			select {
			//TODO: errors should show up on screen
			//      instead of halting loop
			//See: https://github.com/buildpacks/pack/issues/1262
			case err := <-copyErr:
				return err
			case err := <-errChan:
				return err
			default:
				if !scanner.Scan() {
					err := scanner.Err()
					if err != nil {
						return err
					}

					return nil
				}

				text := scanner.Text()

				switch {
				case strings.Contains(text, "===> ANALYZING"):
					s.currentPage.Stop()
				default:
					// no-op
				}
			}
		}
	}
}
