package logging

import (
	"fmt"
	"io"
	"time"
)

// TimeWriter prepends the time onto log messages
type TimeWriter struct {
	out   io.Writer
	clock func() time.Time
}

// NewTimeWriter creates a TimeWriter
func NewTimeWriter(writer io.Writer, clock func() time.Time) *TimeWriter {
	return &TimeWriter{
		out:   writer,
		clock: clock,
	}
}

// Write writes a message prepended by the time to the set io.Writer
func (tw *TimeWriter) Write(buf []byte) (n int, err error) {
	ts := tw.clock().Format(timeFmt)
	_, err = fmt.Fprint(tw.out, appendMissingLineFeed(fmt.Sprintf("%s %s", ts, buf)))
	return len(buf), err
}
