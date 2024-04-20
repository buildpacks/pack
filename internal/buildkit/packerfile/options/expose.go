package options

// NOTE: All the Options provided here might not work!

type protocol string

const (
	TCP = protocol("tcp")
	UDP = protocol("udp")
)

// The EXPOSE instruction informs Docker that the container listens on the
// specified network ports at runtime. supported [TCP or UDP]. Defaults to [TCP]
type EXPOSEOptions struct {
	Port     string   // REQUIRED.
	Protocol protocol // OPTIONAL.
}
