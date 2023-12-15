package errors

import "errors"

var (
	ErrDuplicateName      = errors.New("Image/ImageIndex with the given name exists")
	ErrIndexUnknown       = errors.New("cannot find Image Index with the given name")
	ErrNotAddManifestList = errors.New("error while adding ImageIndex to the list")
)