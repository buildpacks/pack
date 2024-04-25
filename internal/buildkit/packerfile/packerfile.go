package packerfile

// Packerfile is an interface that performs set of instructions. Inspired from [Dockerfile] syntax
type Packerfile[T any] interface {
	// ADDCommand(string, options.ADD) error                          // Add local or remote files and directories.
	AddArg(...string) *T // Pass the ARGs to CMD. A key value pair with optional value seperated by `=` delim
	// CMDCommand(cmd []string, ops options.CMD) error                // Specify default commands. There can only be one CMD instruction in a Dockerfile. If you list more than one CMD, only the last one takes effect.
	// COPYCommand(src string, desc []string, ops options.COPY) error // Copy files and directories.
	// ENTRYPOINTCommand(options.ENTRYPOINT) error                    // Specify default executable.
	// ENVCommand(options.ENV) error                                  // Set environment variables.
	// EXPOSECommand(options.EXPOSE, ...options.EXPOSE) error         // Describe which ports your application is listening on.
	// FROMCommand(options.FROM) error                                // Create a new build stage from a base image.
	// HEALTHCHECKCommand(options.HEALTHCHECK) error                  // Check a container's health on startup.
	// LABELCommand(options.LABELOptions, ...options.LABELOptions) error // Add metadata to an image.
	// MAINTAINERCommand(string) error                                       // Deprecated. Specify the author of an image.
	// ONBUILDCommand(options.ONBUILDOptions) error                          // Specify instructions for when the image is used in a build.
	// RUNCommand(cmd []string, ops options.RUNOptions) error // Execute build commands.
	// SHELLCommand(string, ...string) error                                 // Set the default shell of an image.
	// STOPSIGNALCommand(options.STOPSIGNAL) error            // Specify the system call signal for exiting a container.
	// USERCommand(options.USER) error                        // Set user and group ID.
	// VOLUMECommand(options.VOLUME, ...options.VOLUME) error // Create volume mounts.
	// WORKDIRCommand(options.WORKDIR) error                  // Change working directory.
}
