package options

// NOTE: All the Options provided here might not work!

// NOTE: The [ARGOptions] is embedded in [ENVOptions] and [LABELOptions]!
// Remove the embed if any Options specific to [ARG] is added in future.

// The ARG instruction defines a variable that users can pass at build-time to the builder
//
//	with the [pack build] command using the [--env <varname>=<value>] flag.
type ARGOptions struct {
	Key   string
	Value string
}
