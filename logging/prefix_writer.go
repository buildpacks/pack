package logging

import (
	"bufio"
	"bytes"
	"fmt"
	"io"

	"github.com/buildpacks/pack/internal/style"
)

// PrefixWriter is a buffering writer that prefixes each new line. Close should be called to properly flush the buffer.
type PrefixWriter struct {
	out    io.Writer
	buf    *bytes.Buffer
	prefix string
}

// NewPrefixWriter writes by w will be prefixed
func NewPrefixWriter(w io.Writer, prefix string) *PrefixWriter {
	return &PrefixWriter{
		out:    w,
		prefix: fmt.Sprintf("[%s] ", style.Prefix(prefix)),
		buf:    &bytes.Buffer{},
	}
}

// Write writes bytes to the embedded log function
func (w *PrefixWriter) Write(data []byte) (int, error) {
	scanner := bufio.NewScanner(bytes.NewReader(data))
	scanner.Split(ScanLinesKeepNewLine)
	for scanner.Scan() {
		newBits := scanner.Bytes()
		if newBits[len(newBits)-1] != '\n' { // just append if we don't have a new line
			_, err := w.buf.Write(newBits)
			if err != nil {
				return 0, err
			}
		} else { // write our complete message
			var allBits []byte
			if w.buf.Len() > 0 {
				allBits = append(w.buf.Bytes(), newBits...)
				w.buf.Reset()
			} else {
				allBits = newBits
			}

			err := w.writeWithPrefix(allBits)
			if err != nil {
				return 0, err
			}
		}
	}

	return len(data), nil
}

// Close writes any pending data in the buffer
func (w *PrefixWriter) Close() error {
	if w.buf.Len() > 0 {
		err := w.writeWithPrefix(w.buf.Bytes())
		if err != nil {
			return err
		}
	}

	w.buf.Reset()

	return nil
}

func (w *PrefixWriter) writeWithPrefix(bits []byte) error {
	_, err := fmt.Fprint(w.out, w.prefix+string(bits))
	return err
}

// A customized implementation of bufio.ScanLines that preserves new line characters.
func ScanLinesKeepNewLine(data []byte, atEOF bool) (advance int, token []byte, err error) {
	if atEOF && len(data) == 0 {
		return 0, nil, nil
	}
	if i := bytes.IndexByte(data, '\n'); i >= 0 {
		// We have a full newline-terminated line.
		return i + 1, append(dropCR(data[0:i]), '\n'), nil
	}
	// If we're at EOF, we have a final, non-terminated line. Return it.
	if atEOF {
		return len(data), dropCR(data), nil
	}
	// Request more data.
	return 0, nil, nil
}

// dropCR drops a terminal \r from the data.
func dropCR(data []byte) []byte {
	if len(data) > 0 && data[len(data)-1] == '\r' {
		return data[0 : len(data)-1]
	}
	return data
}
