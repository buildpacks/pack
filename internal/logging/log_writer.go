package logging

import (
	"fmt"
	"io"
	"sync"
	"time"
)

// LogWriter is a writer used for logs
type LogWriter struct {
	sync.Mutex
	out      io.Writer
	clock    func() time.Time
	wantTime bool
}

// NewLogWriter creates a LogWriter
func NewLogWriter(writer io.Writer, clock func() time.Time, wantTime bool) *LogWriter {
	return &LogWriter{
		out:      writer,
		clock:    clock,
		wantTime: wantTime,
	}
}

// Write writes a message prepended by the time to the set io.Writer
func (tw *LogWriter) Write(buf []byte) (n int, err error) {
	tw.Lock()
	defer tw.Unlock()

	prefix := ""
	if tw.wantTime {
		prefix = fmt.Sprintf("%s ", tw.clock().Format(timeFmt))
	}

	_, err = fmt.Fprint(tw.out, appendMissingLineFeed(fmt.Sprintf("%s%s", prefix, buf)))
	return len(buf), err
}
