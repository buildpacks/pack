// Package logging implements the logger for the pack CLI.
package logging

import (
	"fmt"
	"io"
	"io/ioutil"
	"sync"
	"time"

	"github.com/apex/log"
	"golang.org/x/term"

	"github.com/buildpacks/pack/internal/style"
	"github.com/buildpacks/pack/logging"
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
	// InvalidFileDescriptor based on https://golang.org/src/os/file_unix.go?s=2183:2210#L57
	InvalidFileDescriptor = ^(uintptr(0))
)

// LogWithWriters is a logger used with the pack CLI, allowing users to print logs for various levels, including Info, Debug and Error
type LogWithWriters struct {
	sync.Mutex
	log.Logger
	wantTime bool
	clock    func() time.Time
	out      io.Writer
	errOut   io.Writer
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

// WithClock is an option used to initialize a LogWithWriters with a given clock function
func WithClock(clock func() time.Time) func(writers *LogWithWriters) {
	return func(logger *LogWithWriters) {
		logger.clock = clock
	}
}

// WithVerbose is an option used to initialize a LogWithWriters with Verbose turned on
func WithVerbose() func(writers *LogWithWriters) {
	return func(logger *LogWithWriters) {
		logger.Level = log.DebugLevel
	}
}

// HandleLog handles log events, printing entries appropriately
func (lw *LogWithWriters) HandleLog(e *log.Entry) error {
	lw.Lock()
	defer lw.Unlock()

	writer := lw.WriterForLevel(logging.Level(e.Level))
	_, err := fmt.Fprint(writer, appendMissingLineFeed(fmt.Sprintf("%s%s", formatLevel(e.Level), e.Message)))

	return err
}

// WriterForLevel returns a Writer for the given logging.Level
func (lw *LogWithWriters) WriterForLevel(level logging.Level) io.Writer {
	if lw.Level > log.Level(level) {
		return ioutil.Discard
	}

	if level == logging.ErrorLevel {
		return NewLogWriter(lw.errOut, lw.clock, lw.wantTime)
	}

	return NewLogWriter(lw.out, lw.clock, lw.wantTime)
}

// Writer returns the base Writer for the LogWithWriters
func (lw *LogWithWriters) Writer() io.Writer {
	return lw.out
}

// WantTime turns timestamps on in log entries
func (lw *LogWithWriters) WantTime(f bool) {
	lw.wantTime = f
}

// WantQuiet reduces the number of logs returned
func (lw *LogWithWriters) WantQuiet(f bool) {
	if f {
		lw.Level = quietLevel
	}
}

// WantVerbose increases the number of logs returned
func (lw *LogWithWriters) WantVerbose(f bool) {
	if f {
		lw.Level = verboseLevel
	}
}

// IsVerbose returns whether verbose logging is on
func (lw *LogWithWriters) IsVerbose() bool {
	return lw.Level == log.DebugLevel
}

// IsTerminal returns whether a writer is a terminal
func IsTerminal(w io.Writer) (uintptr, bool) {
	if f, ok := w.(hasDescriptor); ok {
		termFd := f.Fd()
		isTerm := term.IsTerminal(int(termFd))
		return termFd, isTerm
	}

	return InvalidFileDescriptor, false
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

// Preserve behavior of other loggers
func appendMissingLineFeed(msg string) string {
	buff := []byte(msg)
	if len(buff) == 0 || buff[len(buff)-1] != lineFeed {
		buff = append(buff, lineFeed)
	}
	return string(buff)
}
