package options

import "time"

// NOTE: All the Options provided here might not work!

// HEALTHCHECK option used by [Dockerfile HEALTHCHECH] instruction.
type HEALTHCHECK struct {
	NONE               bool // disable HEALTHCHECK. Cannot be used with other OPTIONS!
	HEALTHCHECKEnabled      // enable HEALTHCHECH. Cannot be used with other OPTIONS!
}

type HEALTHCHECKEnabled struct {
	Interval      time.Duration
	Timeout       time.Duration
	StartPeriod   time.Duration
	StartInterval time.Duration
	Retries       uint64
	CMD           func(cmd []string, ops CMD)
}
