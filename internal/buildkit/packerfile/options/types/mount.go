package types

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

type BindMountOptions MountOption
type CacheMountOptions MountOption
type TmpFSMountOptions MountOption
type SecretMountOptions MountOption
type SSHMountOptions MountOption
