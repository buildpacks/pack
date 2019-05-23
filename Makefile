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
ifeq ($(PACK_CI), true)
		$(eval files := $(shell goimports -l -local github.com/buildpack/pack $(shell find . -type f -name '*.go' -not -path "./vendor/*")))
		@if [[ "$(files)" ]]; then \
			echo "The following files have imports that must be reordered:\n $(files)"; \
			exit 1; \
		fi
else
		goimports -l -w -local github.com/buildpack/pack $$(find . -type f -name '*.go' -not -path "./vendor/*")
endif

vet:
	$(GOCMD) vet $$($(GOCMD) list ./... | grep -v /testdata/)

test: format imports vet unit acceptance
	
unit: format imports vet
	$(GOCMD) test -mod=vendor -v -count=1 -parallel=1 -timeout=0 ./...
	
acceptance: format imports vet
	$(GOCMD) test -mod=vendor -v -count=1 -parallel=1 -timeout=0 -tags=acceptance ./acceptance

clean:
	rm -rf ./out

.PHONY: clean build format imports vet test unit acceptance