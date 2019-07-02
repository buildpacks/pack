# Go parameters
GOCMD=go
GOENV=CGO_ENABLED=0
PACK_VERSION?=dev
PACK_BIN?=pack

all: clean test build

build: format imports vet
	mkdir -p ./out
	$(GOENV) $(GOCMD) build -mod=vendor -ldflags "-X 'main.Version=${PACK_VERSION}'" -o ./out/$(PACK_BIN) -a ./cmd/pack

format:
ifeq ($(PACK_CI), true)
		test -z $$($(GOCMD) fmt ./...)
else
		$(GOCMD) fmt ./...
endif

imports:
	go install -mod=vendor golang.org/x/tools/cmd/goimports
ifeq ($(PACK_CI), true)
		test -z $$(goimports -l -local github.com/buildpack/pack $$(find . -type f -name '*.go' -not -path "./vendor/*"))
else
		goimports -l -w -local github.com/buildpack/pack $$(find . -type f -name '*.go' -not -path "./vendor/*")
endif

vet:
	$(GOCMD) vet -mod=vendor $$($(GOCMD) list ./... | grep -v /testdata/)

test: unit acceptance

unit: format imports vet
	$(GOCMD) test -mod=vendor -v -count=1 -parallel=1 -timeout=0 ./...

acceptance: format imports vet
	$(GOCMD) test -mod=vendor -v -count=1 -parallel=1 -timeout=0 -tags=acceptance ./acceptance

clean:
	rm -rf ./out

.PHONY: clean build format imports vet test unit acceptance