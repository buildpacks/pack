package dockerfile

import "github.com/buildpacks/pack/internal/buildkit/dockerfile/options"

// Dockerfile is an interface with a set of instructions that are supported by a [Dockerfile]
type Dockerfile interface {
	ADDCommand(string, options.ADDOptions) error                          // Add local or remote files and directories.
	ARGCommand(options.ARGOptions) error                                  // Use build-time variables.
	CMDCommand(cmd []string, ops options.CMDOptions) error                // Specify default commands. There can only be one CMD instruction in a Dockerfile. If you list more than one CMD, only the last one takes effect.
	COPYCommand(src string, desc []string, ops options.COPYOptions) error // Copy files and directories.
	ENTRYPOINTCommand(options.ENDPOINTOptions) error                      // Specify default executable.
	ENVCommand(options.ENVOption) error                                   // Set environment variables.
	EXPOSECommand(options.EXPOSEOptions, ...options.EXPOSEOptions) error  // Describe which ports your application is listening on.
	FROMCommand(options.FROMOptions) error                                // Create a new build stage from a base image.
	HEALTHCHECKCommand(options.HEALTHCHECKOptions) error                  // Check a container's health on startup.
	LABELCommand(options.LABELOptions, ...options.LABELOptions) error     // Add metadata to an image.
	MAINTAINERCommand(string) error                                       // Deprecated. Specify the author of an image.
	ONBUILDCommand(options.ONBUILDOptions) error                          // Specify instructions for when the image is used in a build.
	RUNCommand(cmd []string, ops options.RUNOptions) error                // Execute build commands.
	SHELLCommand(string, ...string) error                                 // Set the default shell of an image.
	STOPSIGNALCommand(options.STOPSIGNAL) error                           // Specify the system call signal for exiting a container.
	USERCommand(options.USER) error                                       // Set user and group ID.
	VOLUMECommand(options.VOLUME, ...options.VOLUME) error                // Create volume mounts.
	WORKDIRCommand(options.WORKDIR) error                                 // Change working directory.
}
