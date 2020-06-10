package logging

import (
	"fmt"
	"io"
	"os"
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

	_, err = fmt.Fprintf(tw.out, "%s%s", prefix, buf)
	return len(buf), err
}

// Fd returns the file descriptor of the writer. This is used to ensure it is a Console, and can therefore display streams of text
func (tw *LogWriter) Fd() uintptr {
	tw.Lock()
	defer tw.Unlock()

	file, ok := tw.out.(*os.File)
	if ok {
		return file.Fd()
	}

	return 0
}
