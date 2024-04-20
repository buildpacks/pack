package options

// NOTE: All the Options provided here might not work!

type form int

const (
	SHELL form = iota
	EXEC
)

// NOTE: [CMDOptions] is embedded in [ENTRYPOINTOptions]!
// Remove the embed if any Options specific to [CMD] is added in future.

// You can specify CMD instructions using shell or exec forms
type CMDOptions struct {
	Form form
}
