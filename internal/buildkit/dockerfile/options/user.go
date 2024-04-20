package options

type UID uint64
type GID uint64

type USER struct {
	UID // REQUIRED.
	GID // OPTIONAL.
}
