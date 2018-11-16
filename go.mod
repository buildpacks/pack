module github.com/buildpack/pack

require (
	github.com/BurntSushi/toml v0.3.1
	github.com/Nvveen/Gotty v0.0.0-20120604004816-cd527374f1e5 // indirect
	github.com/buildpack/lifecycle v0.0.0-20181116214402-fa66d9071434
	github.com/buildpack/packs v0.0.0-20180824001031-aa30a412923763df37e83f14a6e4e0fe07e11f25
	github.com/docker/docker v0.7.3-0.20181027010111-b8e87cfdad8d
	github.com/docker/go-connections v0.4.0
	github.com/golang/mock v1.1.1
	github.com/google/go-cmp v0.2.0
	github.com/google/go-containerregistry v0.0.0-20181023232207-eb57122f1bf9
	github.com/google/uuid v0.0.0-20171129191014-dec09d789f3d
	github.com/gorilla/mux v1.6.2 // indirect
	github.com/pkg/errors v0.8.0
	github.com/sclevine/spec v1.0.0
	github.com/spf13/cobra v0.0.3
	github.com/spf13/pflag v1.0.3 // indirect
	golang.org/x/net v0.0.0-20181023162649-9b4f9f5ad519 // indirect
	golang.org/x/sys v0.0.0-20181025063200-d989b31c8746 // indirect
	google.golang.org/grpc v1.16.0 // indirect
)

replace github.com/google/go-containerregistry v0.0.0-20181023232207-eb57122f1bf9 => github.com/dgodd/go-containerregistry v0.0.0-20180912122137-611aad063148a69435dccd3cf8475262c11814f6
