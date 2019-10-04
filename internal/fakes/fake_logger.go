package fakes

import (
	"fmt"
	"io"

	"github.com/apex/log"
)

type fakeLog struct {
	log.Logger
	w io.Writer
}

func (f *fakeLog) WantLevel(level string) {
	//do nothing
}

func NewFakeLogger(w io.Writer) *fakeLog { //nolint:golint,gosimple
	f := &fakeLog{
		w: w,
	}
	f.Logger.Handler = f
	f.Logger.Level = log.DebugLevel
	return f
}

func (f *fakeLog) HandleLog(e *log.Entry) error {
	switch e.Level {
	case log.WarnLevel:
		_, _ = fmt.Fprintf(f.w, "Warning: %s\n", e.Message)
	case log.ErrorLevel:
		_, _ = fmt.Fprintf(f.w, "ERROR: %s\n", e.Message)
	default:
		_, _ = fmt.Fprintln(f.w, e.Message)
	}

	return nil
}

func (f *fakeLog) Writer() io.Writer {
	return f.w
}

func (f *fakeLog) IsVerbose() bool {
	return false
}
