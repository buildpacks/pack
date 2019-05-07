package logging

import (
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/apex/log"
)

// Terminal colors
const (
	red                    = 31
	yellow                 = 33
	blue                   = 34
	gray                   = 37
)

// std time format
const timeFmt = "2006/01/02 15:04:05.000000"

// Colors map to log levels
var Colors = [...]int{
	log.DebugLevel: gray,
	log.InfoLevel:  blue,
	log.WarnLevel:  yellow,
	log.ErrorLevel: red,
	log.FatalLevel: red,
}

// Strings mapping.
var Strings = [...]string{
	log.DebugLevel: "DEBUG",
	log.InfoLevel:  "INFO",
	log.WarnLevel:  "WARN",
	log.ErrorLevel: "ERROR",
	log.FatalLevel: "FATAL",
}

// Handler implementation.
type Handler struct {
	sync.Mutex
	Writer   io.Writer
	WantTime bool
	NoColor  bool
	timer    func() time.Time
}

func formatLevel(level log.Level, noColor bool ) string {
	if noColor {
		return fmt.Sprintf("%-6s", Strings[level])
	}

	return fmt.Sprintf("\033[%dm%-6s\033[0m", Colors[level], Strings[level] )
}

// HandleLog supports behavior that is unique to pack, namely toggling colors and timestamps.
func (h *Handler) HandleLog(e *log.Entry) error {
	h.Lock()
	defer h.Unlock()

	if h.WantTime {
		ts := h.timer().Format(timeFmt)
		_, _ = fmt.Fprintf(h.Writer, "%s %s %-25s", ts, formatLevel(e.Level, h.NoColor), e.Message)
	} else {
		_, _ = fmt.Fprintf(h.Writer, "%s %-25s", formatLevel(e.Level, h.NoColor), e.Message)
	}

	_, _ = fmt.Fprintln(h.Writer)

	return nil
}


// NewLogHandler creates a pack cli specific logger
func NewLogHandler(w io.Writer) *Handler {
	return &Handler{
		Writer:  w,
		timer: func() time.Time {
			return time.Now()
		},
	}
}

type logWithWriter struct {
	log.Logger
	w io.Writer
}

func(lw *logWithWriter) Writer() io.Writer {
	return lw.w
}

// NewLogWithWriter creates a logger that implements io.Writer
func NewLogWithWriter(h *Handler) *logWithWriter {
	var lw logWithWriter
	lw.w = h.Writer
	lw.Logger.Handler = h
	lw.Logger.Level = log.DebugLevel
	return &lw
}

