// Package logging implements the logger for the pack CLI.
package logging

import (
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
	if len(buff) == 0 || buff[len(buff)-1] != lineFeed {
		buff = append(buff, lineFeed)
	}
	return string(buff)
}

// HandleLog supports behavior that is unique to Pack CLI, namely toggling colors and timestamps.
func (h *handler) HandleLog(e *log.Entry) error {
	h.Lock()
	defer h.Unlock()

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

func (lw *logWithWriters) WantLevel(level string) {
	//do nothing
}

func (lw *logWithWriters) Writer() io.Writer {
	return lw.out
}

// DebugInfoWriter - returns stderr if log level is not set to quiet.
func (lw *logWithWriters) InfoErrorWriter() io.Writer {
	if lw.Level == log.InfoLevel ||
		lw.Level == log.DebugLevel {
		return lw.errOut
	}
	return ioutil.Discard
}

// InfoWriter returns stdout if logging is not set to quiet.
func (lw *logWithWriters) InfoWriter() io.Writer {
	if lw.Level == log.InfoLevel ||
		lw.Level == log.DebugLevel {
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
	}
}

func (lw *logWithWriters) WantVerbose(f bool) {
	if f {
		lw.Level = log.DebugLevel
	}
}

func (lw *logWithWriters) IsVerbose() bool {
	return lw.Level == log.DebugLevel
}

// NewLogWithWriters creates a logger to be used with pack CLI.
func NewLogWithWriters(stdout, stderr io.Writer) *logWithWriters { //nolint:golint,gosimple
	hnd := &handler{
		writer: stdout,
		timer:  time.Now,
	}
	var lw logWithWriters
	lw.handler = hnd
	lw.out = hnd.writer
	lw.errOut = stderr
	lw.Logger.Handler = hnd
	lw.Logger.Level = log.InfoLevel
	return &lw
}
