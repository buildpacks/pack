export GO111MODULE = on

GOCMD?=go
GOENV=GOOS=linux GOARCH=amd64 CGO_ENABLED=0
GOBUILD=$(GOCMD) build -mod=vendor -ldflags "-X 'github.com/buildpack/lifecycle/cmd.Version=$(LIFECYCLE_VERSION)' -X 'github.com/buildpack/lifecycle/cmd.SCMRepository=$(SCM_REPO)' -X 'github.com/buildpack/lifecycle/cmd.SCMCommit=$(SCM_COMMIT)'"
GOTEST=$(GOCMD) test -mod=vendor
LIFECYCLE_VERSION?=0.0.0
PLATFORM_API=0.1
BUILDPACK_API=0.2
SCM_REPO?=
SCM_COMMIT=$$(git rev-parse --short HEAD)
ARCHIVE_NAME=lifecycle-v$(LIFECYCLE_VERSION)+linux.x86-64

define LIFECYCLE_DESCRIPTOR
[api]
  platform = "$(PLATFORM_API)"
  buildpack = "$(BUILDPACK_API)"

[lifecycle]
  version = "$(LIFECYCLE_VERSION)"
endef

all: test build package

build:
	@echo "> Building lifecycle..."
	mkdir -p ./out/$(ARCHIVE_NAME)
	$(GOENV) $(GOBUILD) -o ./out/lifecycle/detector -a ./cmd/detector
	$(GOENV) $(GOBUILD) -o ./out/lifecycle/restorer -a ./cmd/restorer
	$(GOENV) $(GOBUILD) -o ./out/lifecycle/analyzer -a ./cmd/analyzer
	$(GOENV) $(GOBUILD) -o ./out/lifecycle/builder -a ./cmd/builder
	$(GOENV) $(GOBUILD) -o ./out/lifecycle/exporter -a ./cmd/exporter
	$(GOENV) $(GOBUILD) -o ./out/lifecycle/cacher -a ./cmd/cacher
	$(GOENV) $(GOBUILD) -o ./out/lifecycle/launcher -a ./cmd/launcher
	$(GOENV) $(GOBUILD) -o ./out/lifecycle/rebaser -a ./cmd/rebaser

descriptor: export LIFECYCLE_DESCRIPTOR:=$(LIFECYCLE_DESCRIPTOR)
descriptor:
	@echo "> Writing descriptor file..."
	mkdir -p ./out
	echo "$${LIFECYCLE_DESCRIPTOR}" > ./out/lifecycle.toml


install-goimports:
	@echo "> Installing goimports..."
	$(GOCMD) install -mod=vendor golang.org/x/tools/cmd/goimports

install-yj:
	@echo "> Installing yj..."
	$(GOCMD) install -mod=vendor github.com/sclevine/yj

install-mockgen:
	@echo "> Installing mockgen..."
	$(GOCMD) install -mod=vendor github.com/golang/mock/mockgen

generate: install-mockgen
	@echo "> Generating..."
	$(GOCMD) generate

format: install-goimports
	@echo "> Formating code..."
	test -z $$(goimports -l -w -local github.com/buildpack/lifecycle $$(find . -type f -name '*.go' -not -path "./vendor/*"))

vet:
	@echo "> Vetting code..."
	$(GOCMD) vet -mod=vendor $$($(GOCMD) list -mod=vendor ./... | grep -v /testdata/)

test: unit acceptance

unit: format vet install-yj
	@echo "> Running unit tests..."
	$(GOTEST) -v -count=1 ./...

acceptance: format vet
	@echo "> Running acceptance tests..."
	$(GOTEST) -v -count=1 -tags=acceptance ./acceptance/...

clean:
	@echo "> Cleaning workspace..."
	rm -rf ./out

package: descriptor
	@echo "> Packaging lifecycle..."
	tar czf ./out/$(ARCHIVE_NAME).tgz -C out lifecycle.toml lifecycle