package types

type Security string

// The default security mode is sandbox. With --security=insecure, the builder runs the command without sandbox in insecure mode,
// which allows to run flows requiring elevated privileges (e.g. containerd). This is equivalent to running docker run --privileged.
const (
	SANDBOX  = Security("sandbox") // Default.
	INSECURE = Security("insecure")
)
