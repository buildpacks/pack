# Go parameters
GOCMD=go
GOENV=CGO_ENABLED=0
PACK_VERSION?=dev
PACK_BIN?=pack

all: clean test build

build: format vet
	mkdir -p ./out
	$(GOENV) $(GOCMD) build -mod=vendor -ldflags "-X 'main.Version=${PACK_VERSION}'" -o ./out/$(PACK_BIN) -a ./cmd/pack

format:
ifeq ($(PACK_CI), true)
		test -z $$($(GOCMD) fmt ./...)
else
		$(GOCMD) fmt ./...
endif

vet:
	$(GOCMD) vet $$($(GOCMD) list ./... | grep -v /testdata/)

test: format vet unit acceptance
	
unit: format vet
	$(GOCMD) test -mod=vendor -v -count=1 -parallel=1 -timeout=0 ./...
	
acceptance: format vet
	$(GOCMD) test -mod=vendor -v -count=1 -parallel=1 -timeout=0 -tags=acceptance ./acceptance

clean:
	rm -rf ./out

.PHONY: clean build format vet test unit acceptance