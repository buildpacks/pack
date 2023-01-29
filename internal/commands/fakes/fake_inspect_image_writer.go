package fakes

import (
	"github.com/buildpacks/pack/internal/inspectimage"
	"github.com/buildpacks/pack/pkg/client"
	"github.com/buildpacks/pack/pkg/logging"
)

type FakeInspectImageWriter struct {
	PrintForLocal  string
	PrintForRemote string
	ErrorForPrint  error

	ReceivedInfoForLocal               *client.ImageInfo
	ReceivedInfoForRemote              *client.ImageInfo
	ReceivedInfoForLocalWithExtension  *client.ImageWithExtensionInfo
	ReceivedInfoForRemoteWithExtension *client.ImageWithExtensionInfo
	RecievedGeneralInfo                inspectimage.GeneralInfo
	ReceivedErrorForLocal              error
	ReceivedErrorForRemote             error
}

func (w *FakeInspectImageWriter) Print(
	logger logging.Logger,
	sharedInfo inspectimage.GeneralInfo,
	local, remote *client.ImageInfo,
	localWithExtension, remoteWithExtension *client.ImageWithExtensionInfo,
	localErr, remoteErr error,
) error {
	w.ReceivedInfoForLocal = local
	w.ReceivedInfoForRemote = remote
	w.ReceivedInfoForLocalWithExtension = localWithExtension
	w.ReceivedInfoForRemoteWithExtension = remoteWithExtension
	w.ReceivedErrorForLocal = localErr
	w.ReceivedErrorForRemote = remoteErr
	w.RecievedGeneralInfo = sharedInfo

	logger.Infof("\nLOCAL:\n%s\n", w.PrintForLocal)
	logger.Infof("\nREMOTE:\n%s\n", w.PrintForRemote)

	return w.ErrorForPrint
}
