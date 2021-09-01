package termui

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	"github.com/buildpacks/pack/internal/termui/fakes"
	h "github.com/buildpacks/pack/testhelpers"
)

func TestScreen(t *testing.T) {
	spec.Run(t, "Termui", testTermui, spec.Report(report.Terminal{}))
}

func testTermui(t *testing.T, when spec.G, it spec.S) {
	var (
		assert             = h.NewAssertionManager(t)
		eventuallyInterval = 500 * time.Millisecond
		eventuallyDuration = 5 * time.Second
	)

	it("performs the lifecycle", func() {
		var (
			fakeApp             = fakes.NewApp()
			s                   = Termui{app: fakeApp, textChan: make(chan string, 10)}
			r, w                = io.Pipe()
			fakeDockerStdWriter = fakes.NewDockerStdWriter(w)
		)

		defer func() {
			w.Close()
			fakeApp.StopRunning()
		}()
		go s.Run(func() {})
		go s.Handler()(nil, nil, r)

		h.Eventually(t, func() bool {
			return fakeApp.SetRootCallCount == 1
		}, eventuallyInterval, eventuallyDuration)

		currentPage, ok := s.currentPage.(*Detect)
		assert.TrueWithMessage(ok, fmt.Sprintf("expected %T to be assignable to type `*screen.Detect`", s.currentPage))
		assert.TrueWithMessage(fakeApp.DrawCallCount > 0, "expect app.Draw() to be called")
		h.Eventually(t, func() bool {
			return strings.Contains(currentPage.textView.GetText(false), "Detecting")
		}, eventuallyInterval, eventuallyDuration)

		fakeDockerStdWriter.WriteStdoutln(`===> ANALYZING`)
		h.Eventually(t, func() bool {
			return strings.Contains(currentPage.textView.GetText(false), "Detected!")
		}, eventuallyInterval, eventuallyDuration)
	})

	it("performs the lifecycle (when the builder is untrusted)", func() {
		var (
			fakeApp = fakes.NewApp()
			s       = Termui{app: fakeApp, textChan: make(chan string, 10)}
			r, w    = io.Pipe()
		)

		defer func() {
			w.Close()
			fakeApp.StopRunning()
		}()
		go s.Run(func() {})
		go s.Handler()(nil, nil, r)

		h.Eventually(t, func() bool {
			return fakeApp.SetRootCallCount == 1
		}, eventuallyInterval, eventuallyDuration)

		assert.Equal(fakeApp.SetRootCallCount, 1)
		currentPage, ok := s.currentPage.(*Detect)
		assert.TrueWithMessage(ok, fmt.Sprintf("expected %T to be assignable to type `*screen.Detect`", s.currentPage))
		assert.TrueWithMessage(fakeApp.DrawCallCount > 0, "expect app.Draw() to be called")
		h.Eventually(t, func() bool {
			return strings.Contains(currentPage.textView.GetText(false), "Detecting")
		}, eventuallyInterval, eventuallyDuration)

		s.Info(`===> ANALYZING`)
		h.Eventually(t, func() bool {
			return strings.Contains(currentPage.textView.GetText(false), "Detected!")
		}, eventuallyInterval, eventuallyDuration)
	})

	// TODO: change to show errors on-screen
	// See: https://github.com/buildpacks/pack/issues/1262
	it("returns errors from error channel", func() {
		var (
			errChan = make(chan error, 1)
			fakeApp = fakes.NewApp()
			s       = Termui{app: fakeApp}
		)

		errChan <- errors.New("some-error")

		err := s.Handler()(nil, errChan, bytes.NewReader(nil))
		assert.ErrorContains(err, "some-error")
	})
}
