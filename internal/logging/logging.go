// Package logging implements the logger for the pack CLI.
package logging

import (
	"fmt"
	"io"
	"io/ioutil"
	"sync"
	"time"

	"github.com/apex/log"

	"github.com/buildpacks/pack/internal/style"
)

const (
	errorLevelText = "ERROR: "
	warnLevelText  = "Warning: "
	lineFeed       = '\n'
	// log level to use when quiet is true
	quietLevel = log.WarnLevel
	// log level to use when debug is true
	verboseLevel = log.DebugLevel
	// time format the out logging uses
	timeFmt = "2006/01/02 15:04:05.000000"
)

type LogWithWriters struct {
	sync.Mutex
	log.Logger
	wantTime bool
	clock    func() time.Time
	out      io.Writer
	errOut   io.Writer
}

func WithClock(clock func() time.Time) func(writers *LogWithWriters) {
	return func(logger *LogWithWriters) {
		logger.clock = clock
	}
}

// NewLogWithWriters creates a logger to be used with pack CLI.
func NewLogWithWriters(stdout, stderr io.Writer, opts ...func(*LogWithWriters)) *LogWithWriters {
	lw := &LogWithWriters{
		Logger: log.Logger{
			Level: log.InfoLevel,
		},
		wantTime: false,
		clock:    time.Now,
		out:      stdout,
		errOut:   stderr,
	}
	lw.Logger.Handler = lw

	for _, opt := range opts {
		opt(lw)
	}

	return lw
}

func (lw *LogWithWriters) HandleLog(e *log.Entry) error {
	lw.Lock()
	defer lw.Unlock()

	if lw.Level > e.Level {
		return nil
	}

	writer := lw.out
	if e.Level == log.ErrorLevel {
		writer = lw.errOut
	}

	if lw.wantTime {
		ts := lw.clock().Format(timeFmt)
		_, _ = fmt.Fprint(writer, appendMissingLineFeed(fmt.Sprintf("%s %s%s", ts, formatLevel(e.Level), e.Message)))
		return nil
	}

	_, _ = fmt.Fprint(writer, appendMissingLineFeed(fmt.Sprintf("%s%s", formatLevel(e.Level), e.Message)))

	return nil
}

func (lw *LogWithWriters) Writer() io.Writer {
	return lw.out
}

// ErrorWriter returns the writer for error messages.
func (lw *LogWithWriters) ErrorWriter() io.Writer {
	return lw.errOut
}

// OutWriter returns the writer for standard messages.
func (lw *LogWithWriters) OutWriter() io.Writer {
	if lw.Level >= quietLevel {
		return ioutil.Discard
	}

	return lw.out
}

func (lw *LogWithWriters) WantTime(f bool) {
	lw.wantTime = f
}

func (lw *LogWithWriters) WantQuiet(f bool) {
	if f {
		lw.Level = quietLevel
	}
}

func (lw *LogWithWriters) WantVerbose(f bool) {
	if f {
		lw.Level = verboseLevel
	}
}

func (lw *LogWithWriters) IsVerbose() bool {
	return lw.Level == log.DebugLevel
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
