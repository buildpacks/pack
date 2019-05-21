// Package logging implements the logger for the pack CLI.
package logging

import (
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"sync"
	"time"

	"github.com/apex/log"
	"github.com/fatih/color"

	"github.com/buildpack/pack/style"
)

// Terminal colors
const (
	errorLevelText = "ERROR"
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
	if ll == log.ErrorLevel {
		if color.NoColor {
			return fmt.Sprintf("%-6s ", errorLevelText)
		}
		return style.Error("%-6s ", errorLevelText)
	}
	return ""
}

// preserve behavior of other loggers
func appendMissingLineFeed( msg string ) string {
	buff := []byte(msg)
	if buff[len(buff)-1] != '\n' {
		buff = append(buff, '\n')
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
		_, _ = fmt.Fprint(h.writer, appendMissingLineFeed(fmt.Sprintf("%s %s%-25s", ts, formatLevel(e.Level), e.Message)))
		return nil
	}

	_, _ = fmt.Fprint(h.writer, appendMissingLineFeed(fmt.Sprintf( "%s%-25s", formatLevel(e.Level), e.Message)))

	return nil
}

type logWithWriters struct {
	log.Logger
	out io.Writer
	errOut io.Writer
	handler *handler
}

// Writer returns stdout if level is verbose (debug)
func(lw *logWithWriters) Writer() io.Writer {
	if lw.Level == log.DebugLevel {
		return lw.out
	}
	return ioutil.Discard
}

// ErrorWriter - returns stderr if level is verbose (debug)
func(lw *logWithWriters) ErrorWriter() io.Writer {
	if lw.Level == log.DebugLevel {
		return lw.errOut
	}
	return ioutil.Discard
}

func(lw *logWithWriters) WantTime(f bool) {
	lw.handler.wantTime = f
}

func(lw *logWithWriters) WantQuiet(f bool) {
	lw.Level = log.InfoLevel
}

// NewLogWithWriters creates a logger to be used with pack CLI.
func NewLogWithWriters() *logWithWriters {
	hnd := &handler{
		writer: os.Stdout,
		timer: func() time.Time {
			return time.Now()
		},
	}
	var lw logWithWriters
	lw.handler = hnd
	lw.out = hnd.writer
	lw.errOut = os.Stderr
	lw.Logger.Handler = hnd
	lw.Logger.Level = log.DebugLevel
	return &lw
}



