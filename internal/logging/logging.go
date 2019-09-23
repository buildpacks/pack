// Package logging implements the logger for the pack CLI.
package logging

import (
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"sync"
	"time"

	"github.com/apex/log"

	"github.com/buildpack/pack/style"
)

const (
	errorLevelText = "ERROR: "
	warnLevelText  = "Warning: "
	lineFeed       = '\n'

	// time format the out logging uses
	timeFmt = "2006/01/02 15:04:05.000000"
)

// handler implementation.
type handler struct {
	sync.Mutex
	writer   io.Writer
	wantTime bool
	timer    func() time.Time
}

func formatLevel(ll log.Level) string {
	switch ll {
	case log.ErrorLevel:
		return style.Error(errorLevelText)
	case log.WarnLevel:
		return style.Warn(warnLevelText)
	}

	return ""
}

// preserve behavior of other loggers
func appendMissingLineFeed(msg string) string {
	buff := []byte(msg)
	if buff[len(buff)-1] != lineFeed {
		buff = append(buff, lineFeed)
	}
	return string(buff)
}

// HandleLog supports behavior that is unique to Pack CLI, namely toggling colors and timestamps.
func (h *handler) HandleLog(e *log.Entry) error {
	h.Lock()
	defer h.Unlock()

	// if we have a blank line we don't want padding or prefixes
	if e.Message == "" {
		_, _ = fmt.Fprintln(h.writer)
		return nil
	}

	if h.wantTime {
		ts := h.timer().Format(timeFmt)
		_, _ = fmt.Fprint(h.writer, appendMissingLineFeed(fmt.Sprintf("%s %s%s", ts, formatLevel(e.Level), e.Message)))
		return nil
	}

	_, _ = fmt.Fprint(h.writer, appendMissingLineFeed(fmt.Sprintf("%s%s", formatLevel(e.Level), e.Message)))

	return nil
}

type logWithWriters struct {
	log.Logger
	out     io.Writer
	errOut  io.Writer
	handler *handler
}

func (lw *logWithWriters) Writer() io.Writer {
	return lw.out
}

// DebugInfoWriter - returns stderr if log level is not set to quiet.
func (lw *logWithWriters) InfoErrorWriter() io.Writer {
	if lw.Level == log.InfoLevel {
		return lw.errOut
	}
	return ioutil.Discard
}

// InfoWriter returns stdout if logging is not set to quiet.
func (lw *logWithWriters) InfoWriter() io.Writer {
	if lw.Level == log.InfoLevel {
		return lw.out
	}
	return ioutil.Discard
}

func (lw *logWithWriters) WantTime(f bool) {
	lw.handler.wantTime = f
}

func (lw *logWithWriters) WantQuiet(f bool) {
	if f {
		lw.Level = log.WarnLevel
	} else {
		lw.Level = log.InfoLevel
	}
}

func (lw *logWithWriters) WantVerbose(f bool) {
	if f {
		lw.Level = log.DebugLevel
	} else {
		lw.Level = log.InfoLevel
	}
}

func (lw *logWithWriters) IsVerbose() bool {
	return lw.Level == log.DebugLevel
}

// NewLogWithWriters creates a logger to be used with pack CLI.
func NewLogWithWriters(stdout, stderr io.Writer) *logWithWriters {
	hnd := &handler{
		writer: stdout,
		timer: func() time.Time {
			return time.Now()
		},
	}
	var lw logWithWriters
	lw.handler = hnd
	lw.out = hnd.writer
	lw.errOut = stderr
	lw.Logger.Handler = hnd
	lw.Logger.Level = log.InfoLevel
	return &lw
}

// Writer wraps console output file descriptors and will optionally strip ANSI color codes from output.  The reason
// this is needed is because build pack scripts are generally bash shell scripts and are executed on a docker container.
// The output from these scripts often contain color codes and the docker api returns this raw console output to
// pack code.  If pack is running on Windows, the output containing the color codes looks like crap.
type Writer struct {
	sync.Mutex
	buffer bytes.Buffer
	out    io.Writer
}

// New creates writer taking something that implements io.Writer as an argument.
func New(w io.Writer) *Writer {
	return &Writer{
		out: w,
	}
}

func write(w *Writer, b []byte) {
	if len(b) == 0 {
		return
	}
	i := bytes.IndexByte(b, lineFeed)
	if i == -1 {
		w.buffer.Write(b)
		return
	}
	w.buffer.Write(b[:i+1])
	_, _ = fmt.Fprint(w.out, maybeStripColor(w.buffer.Bytes()))
	w.buffer.Reset()
	write(w, b[i+1:])
}

// Write buffered input is written to the underlying io.Writer when a line feed occurs.
func (w *Writer) Write(b []byte) (n int, err error) {
	w.Lock()
	defer w.Unlock()
	n = len(b)
	write(w, b)
	return n, err
}

// Close writes any remaining buffer contents to underlying io.Writer
func (w *Writer) Close() error {
	w.Lock()
	defer w.Unlock()
	if w.buffer.Len() == 0 {
		return nil
	}
	contents := maybeStripColor(w.buffer.Bytes())
	if len(contents) > 0 {
		_, err := fmt.Fprintln(w.out, contents)
		return err
	}
	return nil
}
