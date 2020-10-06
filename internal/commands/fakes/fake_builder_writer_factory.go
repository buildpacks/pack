package fakes

import "github.com/buildpacks/pack/internal/commands"

type FakeBuilderWriterFactory struct {
	ReturnForWriter commands.BuilderWriter
	ErrorForWriter  error

	ReceivedForKind string
}

func (f *FakeBuilderWriterFactory) Writer(kind string) (commands.BuilderWriter, error) {
	f.ReceivedForKind = kind

	return f.ReturnForWriter, f.ErrorForWriter
}
