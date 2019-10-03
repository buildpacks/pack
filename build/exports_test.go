package build

import "github.com/docker/docker/api/types/container"

func GetPhaseHostConfig(phase *Phase) *container.HostConfig {
	return phase.hostConf
}
