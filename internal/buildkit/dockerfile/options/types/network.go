package types

type Network string

const (
	DEFAULT = Network("") // Default.
	HOST    = Network("host")
	NONE    = Network("none")
)
