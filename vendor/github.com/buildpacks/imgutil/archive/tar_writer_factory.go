package archive

import (
	"archive/tar"
	"io"
)

type TarWriter interface {
	WriteHeader(hdr *tar.Header) error
	Write(b []byte) (int, error)
	Close() error
}

type TarWriterFactory interface {
	NewTarWriter(io.Writer) TarWriter
}

var DefaultTarWriterFactory = defaultTarWriterFactory{}

type defaultTarWriterFactory struct{}

func (defaultTarWriterFactory) NewTarWriter(w io.Writer) TarWriter {
	return tar.NewWriter(w)
}
