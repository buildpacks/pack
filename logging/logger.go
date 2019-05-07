// Package logging contains log interface and assorted log related utilities
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
}

// WithWriter would typically be used to return an io.Writer that can be used to write text that is unadorned
// with logging information.
type WithWriter interface {
	Writer() io.Writer
}

type LoggerWithWriter interface {
	Logger
	WithWriter
}

// Writer will wrap a logging method in an io.Writer
type Writer struct {
	logMethod func(string)
	prefix    string
}

// NewWriter takes a log method and creates a Writer. For example
// w := NewWriter(log.Info)
func NewWriter(f func(string)) *Writer {
	return &Writer{
		logMethod: f,
	}
}

// Writes bytes to the embedded log function
func (w *Writer) Write(buf []byte) (int, error) {
	w.logMethod(w.prefix + string(buf))
	return len(buf), nil
}

// WithPrefix prepends prefix to log messages
func (w *Writer) WithPrefix(prefix string) *Writer {
	w.prefix = fmt.Sprintf("%s[%s] ", w.prefix, style.Prefix(prefix))
	return w
}


// Tip is a log method used in pack but we don't want to enforce it on other logging libraries we might use, so
// we provide it as a separate function
func Tip(l Logger, format string, v ...interface{}) {
	l.Infof(style.Tip("Tip: ")+format, v...)
}

func FTip(w io.Writer, format string, v ...interface{}) {
	_, _ = fmt.Fprintf(w, style.Tip("Tip: ")+format, v...)
	_, _ = fmt.Fprintln(w)
}