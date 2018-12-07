module github.com/buildpack/pack

require (
	github.com/BurntSushi/toml v0.3.1
	github.com/buildpack/lifecycle v0.0.0-20181207064842-63d0e308b74ffb51bc6b9b9356a2f1faf9994455
	github.com/buildpack/packs v0.0.0-20180824001031-aa30a412923763df37e83f14a6e4e0fe07e11f25
	github.com/dgodd/dockerdial v1.0.1
	github.com/docker/docker v0.7.3-0.20181027010111-b8e87cfdad8d
	github.com/docker/go-connections v0.4.0
	github.com/golang/mock v1.1.1
	github.com/google/go-cmp v0.2.0
	github.com/google/go-containerregistry v0.0.0-20181023232207-eb57122f1bf9
	github.com/google/uuid v0.0.0-20171129191014-dec09d789f3d
	github.com/pkg/errors v0.8.0
	github.com/sclevine/spec v1.0.0
	github.com/spf13/cobra v0.0.3
	github.com/spf13/pflag v1.0.3 // indirect
)

replace github.com/google/go-containerregistry v0.0.0-20181023232207-eb57122f1bf9 => github.com/dgodd/go-containerregistry v0.0.0-20180912122137-611aad063148a69435dccd3cf8475262c11814f6
