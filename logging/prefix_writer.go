package logging

import (
	"bufio"
	"bytes"
	"fmt"
	"io"

	"github.com/buildpacks/pack/internal/style"
)

// PrefixWriter will prefix writes
type PrefixWriter struct {
	out    io.Writer
	prefix string
}

// NewPrefixWriter writes by w will be prefixed
func NewPrefixWriter(w io.Writer, prefix string) *PrefixWriter {
	return &PrefixWriter{
		out:    w,
		prefix: fmt.Sprintf("[%s] ", style.Prefix(prefix)),
	}
}

// Writes bytes to the embedded log function
func (w *PrefixWriter) Write(buf []byte) (int, error) {
	scanner := bufio.NewScanner(bytes.NewReader(buf))
	for scanner.Scan() {
		_, _ = fmt.Fprintln(w.out, w.prefix+scanner.Text())
	}

	return len(buf), nil
}
