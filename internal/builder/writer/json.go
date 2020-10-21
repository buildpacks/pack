package writer

import (
	"encoding/json"
)

type JSON struct {
	StructuredFormat
}

func NewJSON() BuilderWriter {
	return &JSON{
		StructuredFormat: StructuredFormat{
			MarshalFunc: json.Marshal,
		},
	}
}
