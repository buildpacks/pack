package archive

import (
	"archive/tar"
	"io"
)

type TarWriterFactory interface {
	NewTarWriter(io.Writer) TarWriter
}

type TarWriter interface {
	WriteHeader(hdr *tar.Header) error
	Write(b []byte) (int, error)
	Close() error
}

var DefaultTarWriterFactory = defaultTarWriterFactory{}

type defaultTarWriterFactory struct{}

func (defaultTarWriterFactory) NewTarWriter(w io.Writer) TarWriter {
	return tar.NewWriter(w)
}
