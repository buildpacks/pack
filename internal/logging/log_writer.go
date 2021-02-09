package logging

import (
	"fmt"
	"io"
	"regexp"
	"sync"
	"time"

	"github.com/heroku/color"
)

// LogWriter is a writer used for logs
type LogWriter struct {
	sync.Mutex
	out         io.Writer
	clock       func() time.Time
	wantTime    bool
	wantNoColor bool
}

var colorCodeMatcher = regexp.MustCompile(`\x1b\[[0-9;]*m`)

// NewLogWriter creates a LogWriter
func NewLogWriter(writer io.Writer, clock func() time.Time, wantTime bool) *LogWriter {
	wantNoColor := !color.Enabled()
	return &LogWriter{
		out:         writer,
		clock:       clock,
		wantTime:    wantTime,
		wantNoColor: wantNoColor,
	}
}

// Write writes a message prepended by the time to the set io.Writer
func (tw *LogWriter) Write(buf []byte) (n int, err error) {
	tw.Lock()
	defer tw.Unlock()

	length := len(buf)
	if tw.wantNoColor {
		buf = stripColor(buf)
	}

	prefix := ""
	if tw.wantTime {
		prefix = fmt.Sprintf("%s ", tw.clock().Format(timeFmt))
	}

	_, err = fmt.Fprintf(tw.out, "%s%s", prefix, buf)
	return length, err
}

// Fd returns the file descriptor of the writer. This is used to ensure it is a Console, and can therefore display streams of text
func (tw *LogWriter) Fd() uintptr {
	tw.Lock()
	defer tw.Unlock()

	if file, ok := tw.out.(hasDescriptor); ok {
		return file.Fd()
	}

	return invalidFileDescriptor
}

// Remove all ANSI color information.
func stripColor(b []byte) []byte {
	return colorCodeMatcher.ReplaceAll(b, []byte(""))
}

type hasDescriptor interface {
	Fd() uintptr
}
