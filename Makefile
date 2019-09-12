# Go parameters
GOCMD?=go
GOENV=CGO_ENABLED=0
PACK_VERSION?=dev
PACK_BIN?=pack
PACKAGE_BASE=github.com/buildpack/pack
PACKAGES:=$(shell $(GOCMD) list -mod=vendor ./... | grep -v /testdata/)
SRC:=$(shell find . -type f -name '*.go' -not -path "./vendor/*")
ARCHIVE_NAME=pack-$(PACK_VERSION)

all: clean verify test build

build:
	@echo "> Building..."
	mkdir -p ./out
	$(GOENV) $(GOCMD) build -mod=vendor -ldflags "-X 'github.com/buildpack/pack/cmd.Version=${PACK_VERSION}'" -o ./out/$(PACK_BIN) -a ./cmd/pack

package:
	tar czf ./out/$(ARCHIVE_NAME).tgz -C out/ pack

install-mockgen:
	@echo "> Installing mockgen..."
	$(GOCMD) install -mod=vendor github.com/golang/mock/mockgen

install-goimports:
	@echo "> Installing goimports..."
	$(GOCMD) install -mod=vendor golang.org/x/tools/cmd/goimports

format: install-goimports
	@echo "> Formating code..."
	@goimports -l -w -local ${PACKAGE_BASE} ${SRC}

vet:
	@echo "> Vetting code..."
	@$(GOCMD) vet -mod=vendor ${PACKAGES}

test: unit acceptance

unit: format vet
	@echo "> Running unit/integration tests..."
	$(GOCMD) test -mod=vendor -v -count=1 -parallel=1 -timeout=0 ./...

acceptance: format vet
	@echo "> Running acceptance tests..."
	$(GOCMD) test -mod=vendor -v -count=1 -parallel=1 -timeout=0 -tags=acceptance ./acceptance

acceptance-all: format vet
	@echo "> Running acceptance tests..."
	ACCEPTANCE_SUITE_CONFIG=$$(cat ./acceptance/testconfig/all.json) $(GOCMD) test -mod=vendor -v -count=1 -parallel=1 -timeout=0 -tags=acceptance ./acceptance

clean:
	@echo "> Cleaning workspace..."
	rm -rf ./out

verify: verify-format vet

generate: install-mockgen
	@echo "> Generating mocks..."
	$(GOCMD) generate -mod=vendor ./...

verify-format: install-goimports
	@echo "> Verifying format..."
	@test -z "$(shell goimports -l -local ${PACKAGE_BASE} ${SRC})";\
	_err=$$?;\
	[ $$_err -ne 0 ] && echo "ERROR: Format verification failed!\n" &&\
	goimports -d -local ${PACKAGE_BASE} ${SRC} && exit $$_err;\
	exit 0;

.PHONY: clean build format imports vet test unit acceptance verify verify-format