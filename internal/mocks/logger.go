package mocks

import (
	"fmt"
	"io"

	"github.com/apex/log"
)

type mockLog struct {
	log.Logger
	w io.Writer
}

// NewMockLogger create a logger to capture output for testing purposes.
func NewMockLogger(w io.Writer) *mockLog {
	ml := &mockLog{
		w: w,
	}
	ml.Logger.Handler = ml
	ml.Logger.Level = log.DebugLevel
	return ml
}

func (ml *mockLog) HandleLog(e *log.Entry) error {
	switch e.Level {
	case log.WarnLevel:
		_, _ = fmt.Fprintf(ml.w, "Warning: %s\n", e.Message)
	case log.ErrorLevel:
		_, _ = fmt.Fprintf(ml.w, "ERROR: %s\n", e.Message)
	default:
		_, _ = fmt.Fprintln(ml.w, e.Message)
	}

	return nil
}

func (ml *mockLog) Writer() io.Writer {
	return ml.w
}
