package fakes

import (
	"github.com/buildpacks/pack"
	"github.com/buildpacks/pack/internal/commands"
	"github.com/buildpacks/pack/internal/config"
	"github.com/buildpacks/pack/logging"
)

type FakeBuilderWriter struct {
	PrintForLocal  string
	PrintForRemote string
	ErrorForPrint  error

	ReceivedInfoForLocal   *pack.BuilderInfo
	ReceivedInfoForRemote  *pack.BuilderInfo
	ReceivedErrorForLocal  error
	ReceivedErrorForRemote error
	ReceivedBuilderInfo    commands.SharedBuilderInfo
	ReceivedLocalRunImages []config.RunImage
}

func (w *FakeBuilderWriter) Print(
	logger logging.Logger,
	localRunImages []config.RunImage,
	local, remote *pack.BuilderInfo,
	localErr, remoteErr error,
	builderInfo commands.SharedBuilderInfo,
) error {
	w.ReceivedInfoForLocal = local
	w.ReceivedInfoForRemote = remote
	w.ReceivedErrorForLocal = localErr
	w.ReceivedErrorForRemote = remoteErr
	w.ReceivedBuilderInfo = builderInfo
	w.ReceivedLocalRunImages = localRunImages

	logger.Infof("\nLOCAL:\n%s\n", w.PrintForLocal)
	logger.Infof("\nREMOTE:\n%s\n", w.PrintForRemote)

	return w.ErrorForPrint
}
