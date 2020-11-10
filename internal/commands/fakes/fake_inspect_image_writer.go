package fakes

import (
	"github.com/buildpacks/pack"
	writer2 "github.com/buildpacks/pack/internal/inspectimage/writer"
	"github.com/buildpacks/pack/logging"
)

type FakeInspectImageWriter struct {
	PrintForLocal  string
	PrintForRemote string
	ErrorForPrint  error

	ReceivedInfoForLocal   *pack.ImageInfo
	ReceivedInfoForRemote  *pack.ImageInfo
	RecievedSharedInfo     *writer2.SharedImageInfo
	ReceivedErrorForLocal  error
	ReceivedErrorForRemote error
}

func (w *FakeInspectImageWriter) Print(
	logger logging.Logger,
	sharedInfo *writer2.SharedImageInfo,
	local, remote *pack.ImageInfo,
	localErr, remoteErr error,
) error {
	w.ReceivedInfoForLocal = local
	w.ReceivedInfoForRemote = remote
	w.ReceivedErrorForLocal = localErr
	w.ReceivedErrorForRemote = remoteErr
	w.RecievedSharedInfo = sharedInfo

	logger.Infof("\nLOCAL:\n%s\n", w.PrintForLocal)
	logger.Infof("\nREMOTE:\n%s\n", w.PrintForRemote)

	return w.ErrorForPrint
}
