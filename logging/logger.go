// Package logging defines the minimal interface that loggers must support to be used by pack.
package logging

import (
	"fmt"
	"io"

	"github.com/buildpack/pack/style"
)

// Logger defines behavior required by a logging package used by pack libraries
type Logger interface {
	Debug(msg string)
	Debugf(fmt string, v ...interface{})
	Info(msg string)
	Infof(fmt string, v ...interface{})
	Error(msg string)
	Errorf(fmt string, v ...interface{})
	Writer() io.Writer
}

// WithErrorWriter is an optional interface for loggers that want to support a separate writer for errors and standard logging.
type WithErrorWriter interface {
	ErrorWriter() io.Writer
}

// GetErrorWriter will return an ErrorWriter, typically stderr if one exists, otherwise the standard logger writer
// will be returned.
func GetErrorWriter(l Logger) io.Writer {
	if er, ok := l.(WithErrorWriter); ok {
		return er.ErrorWriter()
	}
	return l.Writer()
}

// PrefixWriter will prefix writes
type PrefixWriter struct {
	out    io.Writer
	prefix string
}

// NewPrefixWriter takes a writer and creates a PrefixWriter.
func NewPrefixWriter(w io.Writer) *PrefixWriter {
	return &PrefixWriter{
		out: w,
	}
}

// Writes bytes to the embedded log function
func (w *PrefixWriter) Write(buf []byte) (int, error) {
	_, _ = fmt.Fprint(w.out, w.prefix+string(buf))
	return len(buf), nil
}

// WithPrefix prepends prefix to log messages
func (w *PrefixWriter) WithPrefix(prefix string) *PrefixWriter {
	w.prefix = fmt.Sprintf("%s[%s] ", w.prefix, style.Prefix(prefix))
	return w
}

// Tip logs a tip.
func Tip(l Logger, format string, v ...interface{}) {
	l.Infof(style.Tip("Tip: ")+format, v...)
}
