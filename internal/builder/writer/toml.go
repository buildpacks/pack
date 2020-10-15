package writer

import (
	"bytes"

	"github.com/pelletier/go-toml"

	"github.com/buildpacks/pack/internal/commands"
)

type TOML struct {
	StructuredFormat
}

func NewTOML() commands.BuilderWriter {
	return &TOML{
		StructuredFormat: StructuredFormat{
			MarshalFunc: func(v interface{}) ([]byte, error) {
				buf := bytes.NewBuffer(nil)
				err := toml.NewEncoder(buf).Order(toml.OrderPreserve).PromoteAnonymous(false).Encode(v)
				if err != nil {
					return []byte{}, err
				}
				return buf.Bytes(), nil
			},
		},
	}
}
