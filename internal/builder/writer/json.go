package writer

import (
	"encoding/json"

	"github.com/buildpacks/pack/internal/commands"
)

type JSON struct {
	StructuredFormat
}

func NewJSON() commands.BuilderWriter {
	return &JSON{
		StructuredFormat: StructuredFormat{
			MarshalFunc: json.Marshal,
		},
	}
}