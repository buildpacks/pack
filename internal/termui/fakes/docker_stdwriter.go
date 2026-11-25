package fakes

import (
	"encoding/binary"
	"io"
	"time"

	"github.com/moby/moby/api/pkg/stdcopy"
)

const stdWriterPrefixLen = 8

// stdFrameWriter emits the multiplexed stream format consumed by
// stdcopy.StdCopy: an 8-byte header [streamType, 0, 0, 0, uint32be(len)]
// followed by the payload. The new stdcopy package no longer ships a
// NewStdWriter, so the framing is reproduced here for tests.
type stdFrameWriter struct {
	w io.Writer
	t stdcopy.StdType
}

func (f *stdFrameWriter) Write(p []byte) (int, error) {
	header := [stdWriterPrefixLen]byte{}
	header[0] = byte(f.t)
	binary.BigEndian.PutUint32(header[4:], uint32(len(p)))
	if _, err := f.w.Write(header[:]); err != nil {
		return 0, err
	}
	return f.w.Write(p)
}

type DockerStdWriter struct {
	wOut io.Writer
	wErr io.Writer
}

func NewDockerStdWriter(w io.Writer) *DockerStdWriter {
	return &DockerStdWriter{
		wOut: &stdFrameWriter{w: w, t: stdcopy.Stdout},
		wErr: &stdFrameWriter{w: w, t: stdcopy.Stderr},
	}
}

func (w *DockerStdWriter) WriteStdoutln(contents string) {
	w.write(contents+"\n", stdcopy.Stdout)
}

func (w *DockerStdWriter) WriteStderrln(contents string) {
	w.write(contents+"\n", stdcopy.Stderr)
}

func (w *DockerStdWriter) write(contents string, t stdcopy.StdType) {
	switch t {
	case stdcopy.Stdout:
		w.wOut.Write([]byte(contents))
	case stdcopy.Stderr:
		w.wErr.Write([]byte(contents))
	}

	// guard against race conditions
	time.Sleep(time.Millisecond)
}
