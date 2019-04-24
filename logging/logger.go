package logging

import (
	"fmt"
	"io"
	"io/ioutil"
	"log"

	"github.com/fatih/color"

	"github.com/buildpack/pack/style"
)

type Logger struct {
	verbose bool
	out     *logWriter
	err     *logWriter
}

func NewLogger(stdout, stderr io.Writer, verbose, timestamps bool) *Logger {
	return &Logger{
		verbose: verbose,
		out:     newLogWriter(stdout, timestamps),
		err:     newLogWriter(stderr, timestamps),
	}
}

func (l *Logger) printf(w *logWriter, format string, a ...interface{}) {
	w.Write([]byte(fmt.Sprintf(format+"\n", a...)))
}

func (l *Logger) Info(format string, a ...interface{}) {
	l.printf(l.out, format, a...)
}

func (l *Logger) Verbose(format string, a ...interface{}) {
	if l.verbose {
		l.printf(l.out, format, a...)
	}
}

func (l *Logger) Error(format string, a ...interface{}) {
	l.printf(l.err, "\n"+style.Error("ERROR: ")+format, a...)
}

func (l *Logger) Tip(format string, a ...interface{}) {
	l.printf(l.out, style.Tip("Tip: ")+format, a...)
}

func (l *Logger) VerboseWriter() *logWriter {
	if !l.verbose {
		return nullLogWriter
	}
	return l.out
}

func (l *Logger) RawVerboseWriter() io.Writer {
	if !l.verbose {
		return ioutil.Discard
	}
	return l.out.rawOut
}

func (l *Logger) RawWriter() io.Writer {
	return l.out.rawOut
}

func (l *Logger) VerboseErrorWriter() *logWriter {
	if !l.verbose {
		return nullLogWriter
	}
	return l.err
}

type logWriter struct {
	prefix string
	log    *log.Logger
	rawOut io.Writer
}

var nullLogWriter = newLogWriter(ioutil.Discard, false)

func newLogWriter(out io.Writer, timestamps bool) *logWriter {
	flags := 0
	timestampStart := ""
	timestampEnd := ""
	if !color.NoColor {
		// Go logger prefixes appear before timestamp, so insert color start/end sequences around timestamp
		timestampStart = fmt.Sprintf("\x1b[%dm", style.TimestampColorCode)
		timestampEnd = fmt.Sprintf("\x1b[%dm", color.Reset)
	}
	prefix := ""
	if timestamps {
		flags = log.LstdFlags
		prefix = " "
	}

	return &logWriter{
		prefix: timestampEnd + prefix,
		log:    log.New(out, timestampStart, flags),
		rawOut: out,
	}
}

func (w *logWriter) WithPrefix(prefix string) *logWriter {
	return &logWriter{
		log:    w.log,
		prefix: fmt.Sprintf("%s[%s] ", w.prefix, style.Prefix(prefix)),
		rawOut: w.rawOut,
	}
}

func (w *logWriter) Write(p []byte) (n int, err error) {
	w.log.Print(w.prefix + string(p))
	return len(p), nil
}
