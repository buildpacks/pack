module github.com/buildpack/pack

require (
	github.com/BurntSushi/toml v0.3.1
	github.com/buildpack/lifecycle v0.0.0-20181218142129-e9080de8ee4e
	github.com/dgodd/dockerdial v1.0.1
	github.com/docker/docker v0.7.3-0.20181027010111-b8e87cfdad8d
	github.com/docker/go-connections v0.4.0
	github.com/fatih/color v1.7.0
	github.com/golang/mock v1.2.0
	github.com/google/go-cmp v0.2.0
	github.com/google/go-containerregistry v0.0.0-20181023232207-eb57122f1bf9
	github.com/google/uuid v0.0.0-20171129191014-dec09d789f3d
	github.com/mattn/go-colorable v0.0.9 // indirect
	github.com/mattn/go-isatty v0.0.4 // indirect
	github.com/pkg/errors v0.8.0
	github.com/sclevine/spec v1.0.0
	github.com/spf13/cobra v0.0.3
	github.com/spf13/pflag v1.0.3 // indirect
)

replace github.com/google/go-containerregistry v0.0.0-20181023232207-eb57122f1bf9 => github.com/dgodd/go-containerregistry v0.0.0-20180912122137-611aad063148a69435dccd3cf8475262c11814f6
