package types

// Shoul we rename it to KVPair?
type KeyValue[T any] struct {
	Key   T
	Value T
}

type (
	UID uint64
	GID uint64
)
type (
	WORKDIR string
	VOLUME  string
)

type Security string

// The default security mode is sandbox. With --security=insecure, the builder runs the command without sandbox in insecure mode,
// which allows to run flows requiring elevated privileges (e.g. containerd). This is equivalent to running docker run --privileged.
const (
	SANDBOX  = Security("sandbox") // Default.
	INSECURE = Security("insecure")
)

type Protocol string

const (
	TCP = Protocol("tcp")
	UDP = Protocol("udp")
)

type Network string

const (
	DEFAULT = Network("") // Default.
	HOST    = Network("host")
	NONE    = Network("none")
)

type Mount struct {
	Type    MountType
	Options []MountOption
}

type MountType string
type MountOption []KeyValue[string]

const (
	BIND   = MountType("bind") // Default.
	CACHE  = MountType("cache")
	TmpFS  = MountType("tmpfs")
	Secret = MountType("secret")
	SSH    = MountType("ssh")
)

type (
	BindMountOptions   MountOption
	CacheMountOptions  MountOption
	TmpFSMountOptions  MountOption
	SecretMountOptions MountOption
	SSHMountOptions    MountOption
)

type Form int

const (
	SHELL Form = iota
	EXEC
)
