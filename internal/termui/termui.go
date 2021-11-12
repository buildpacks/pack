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

	"github.com/buildpacks/pack/internal/builder"
	"github.com/buildpacks/pack/internal/container"
	"github.com/buildpacks/pack/pkg/dist"
)

var (
	backgroundColor = tcell.NewRGBColor(5, 30, 40)
)

type app interface {
	SetRoot(root tview.Primitive, fullscreen bool) *tview.Application
	Draw() *tview.Application
	QueueUpdateDraw(f func()) *tview.Application
	Run() error
}

type buildr interface {
	BaseImageName() string
	Buildpacks() []dist.BuildpackInfo
	LifecycleDescriptor() builder.LifecycleDescriptor
	Stack() builder.StackMetadata
}

type page interface {
	Handle(txt string)
	Stop()
}

type Termui struct {
	app         app
	bldr        buildr
	currentPage page

	appName       string
	runImageName  string
	exitCode      int64
	textChan      chan string
	buildpackChan chan dist.BuildpackInfo
}

func NewTermui(appName string, bldr *builder.Builder, runImageName string) *Termui {
	return &Termui{
		appName:       appName,
		bldr:          bldr,
		runImageName:  runImageName,
		app:           tview.NewApplication(),
		buildpackChan: make(chan dist.BuildpackInfo, 50),
		textChan:      make(chan string, 50),
	}
}

// Run starts the terminal UI process in the foreground
// and the passed in function in the background
func (s *Termui) Run(funk func()) error {
	go func() {
		funk()
		s.showBuildStatus()
	}()
	go s.handle()
	defer s.stop()

	s.currentPage = NewDetect(s.app, s.buildpackChan, s.bldr)
	return s.app.Run()
}

func (s *Termui) stop() {
	close(s.textChan)
}

func (s *Termui) handle() {
	for txt := range s.textChan {
		switch {
		case strings.Contains(txt, "===> ANALYZING"):
			s.currentPage.Stop()

			s.currentPage = NewDashboard(s.app, s.appName, s.bldr, s.runImageName, collect(s.buildpackChan))
			s.currentPage.Handle(txt)
		default:
			s.currentPage.Handle(txt)
		}
	}
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
			case body := <-bodyChan:
				s.exitCode = body.StatusCode
				return nil
			default:
				if scanner.Scan() {
					s.textChan <- scanner.Text()
					continue
				}

				if err := scanner.Err(); err != nil {
					return err
				}
			}
		}
	}
}

func (s *Termui) showBuildStatus() {
	if s.exitCode == 0 {
		s.textChan <- "[green::b]\n\nBUILD SUCCEEDED"
		return
	}

	s.textChan <- "[red::b]\n\nBUILD FAILED"
}

func collect(buildpackChan chan dist.BuildpackInfo) []dist.BuildpackInfo {
	close(buildpackChan)

	var result []dist.BuildpackInfo
	for txt := range buildpackChan {
		result = append(result, txt)
	}

	return result
}
