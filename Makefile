ifeq ($(OS),Windows_NT)
SHELL:=cmd.exe

# Need BLANK due to makefile parsing of `\\`
# (see: <https://stackoverflow.com/questions/54733231/how-to-escape-a-backslash-in-the-end-to-mean-literal-backslash-in-makefile/54733416#54733416>)
BLANK:=

# Define variable named `/` to represent OS path separator (usable as `$/` in this file)
/:=\$(BLANK)
CAT=type
RMRF=rmdir /q /s
SRC=$(shell dir /q /s /b *.go | findstr /v $/out$/)
GOIMPORTS_DIFF_OPTION="-l" # Windows can't do diff-mode because it's missing the "diff" binary
PACK_BIN?=pack.exe
else
/:=/
CAT=cat
RMRF=rm -rf
SRC=$(shell find . -type f -name '*.go' -not -path "*/out/*")
GOIMPORTS_DIFF_OPTION:="-d"
PACK_BIN?=pack
endif

ACCEPTANCE_TIMEOUT?=$(TEST_TIMEOUT)
ARCHIVE_NAME=pack-$(PACK_VERSION)
GOCMD?=go
GOFLAGS?=
GOTESTFLAGS?=-v -count=1 -parallel=1
PACKAGE_BASE=github.com/buildpacks/pack
PACK_GITSHA1=$(shell git rev-parse --short=7 HEAD)
PACK_VERSION?=0.0.0
TEST_TIMEOUT?=1200s
UNIT_TIMEOUT?=$(TEST_TIMEOUT)
NO_DOCKER?=

# Clean build flags
clean_build := $(strip ${PACK_BUILD})
clean_sha := $(strip ${PACK_GITSHA1})

# Append build number and git sha to version, if not-empty
ifneq ($(and $(clean_build),$(clean_sha)),)
PACK_VERSION:=${PACK_VERSION}+git-${clean_sha}.build-${clean_build}
else ifneq ($(clean_build),)
PACK_VERSION:=${PACK_VERSION}+build-${clean_build}
else ifneq ($(clean_sha),)
PACK_VERSION:=${PACK_VERSION}+git-${clean_sha}
endif

export GOFLAGS:=$(GOFLAGS)
export CGO_ENABLED=0

BINDIR:=/usr/bin/

.DEFAULT_GOAL := build
.PHONY: clean build format imports lint test unit acceptance prepare-for-pr verify verify-format benchmark 

## bUild: Build the program
build: out
	@echo "=====> Building..."
	$(GOCMD) build -ldflags "-s -w -X 'github.com/buildpacks/pack.Version=${PACK_VERSION}' -extldflags ${LDFLAGS}" -trimpath -o ./out/$(PACK_BIN) -a ./cmd/pack

## all: Run clean, verify, test, and build operations
all: clean verify test build

## clean: Clean the workspace
clean:
	@echo "=====> Cleaning workspace..."
	@$(RMRF) .$/out benchmarks.test || (exit 0)

## format: Format the code
format: install-goimports
	@echo "=====> Formatting code..."
	@goimports -l -w -local ${PACKAGE_BASE} ${SRC}
	@go run tools/pedantic_imports/main.go ${PACKAGE_BASE} ${SRC}

## generate: Generate mocks
generate: install-mockgen
	@echo "=====> Generating mocks..."
	$(GOCMD) generate ./...

## lint: Check the code
lint: install-golangci-lint
	@echo "=====> Linting code..."
	@golangci-lint run -c golangci.yaml

## test: Run unit and acceptance tests
test: unit acceptance

## unit: Run unit tests
unit: out
	@echo "=====> Running unit/integration tests..."
	$(GOCMD) test $(GOTESTFLAGS) -timeout=$(UNIT_TIMEOUT) ./...

## acceptance: Run acceptance tests
acceptance: out
	@echo "=====> Running acceptance tests..."
	$(GOCMD) test $(GOTESTFLAGS) -timeout=$(ACCEPTANCE_TIMEOUT) -tags=acceptance ./acceptance

## acceptance-all: Run all acceptance tests
acceptance-all: export ACCEPTANCE_SUITE_CONFIG:=$(shell $(CAT) .$/acceptance$/testconfig$/all.json)
acceptance-all:
	@echo "=====> Running acceptance tests..."
	$(GOCMD) test $(GOTESTFLAGS) -timeout=$(ACCEPTANCE_TIMEOUT) -tags=acceptance ./acceptance

## prepare-for-pr: Run clean, verify, and test operations and check for uncommitted changes
prepare-for-pr: tidy verify test
	@git diff-index --quiet HEAD -- ||\\
	(echo "-----------------" &&\\
	echo "NOTICE: There are some files that have not been committed." &&\\
	echo "-----------------\\n" &&\\
	git status &&\\
	echo "\\n-----------------" &&\\
	echo "NOTICE: There are some files that have not been committed." &&\\
	echo "-----------------\\n"  &&\\
	exit 0)

## verify: Run format and lint checks
verify: verify-format lint

## verify-format: Verify the format
verify-format: install-goimports
	@echo "=====> Verifying format..."
	$(if $(shell goimports -l -local ${PACKAGE_BASE} ${SRC}), @echo ERROR: Format verification failed! && goimports ${GOIMPORTS_DIFF_OPTION} -local ${PACKAGE_BASE} ${SRC} && exit 1)

## benchmark: Run benchmark tests
benchmark: out
	@echo "=====> Running benchmarks"
	$(GOCMD) test -run=^$  -bench=. -benchtime=1s -benchmem -memprofile=./out/bench_mem.out -cpuprofile=./out/bench_cpu.out -tags=benchmarks ./benchmarks/ -v

## package: Package the program
package: out
	tar czf .$/out$/$(ARCHIVE_NAME).tgz -C .$/out$/ $(PACK_BIN)

## install: Install the program to the system
install:
	mkdir -p ${DESTDIR}${BINDIR}
	cp ./out/$(PACK_BIN) ${DESTDIR}${BINDIR}/

## install-mockgen: Used only by apt-get install when installing ubuntu ppa
install-mockgen:
	@echo "=====> Installing mockgen..."
	cd tools && $(GOCMD) install github.com/golang/mock/mockgen

## install-goimports: Install goimports dependency
install-goimports:
	@echo "=====> Installing goimports..."
	cd tools && $(GOCMD) install golang.org/x/tools/cmd/goimports

## install-golangci-lint: Install golangci-lint dependency
install-golangci-lint:
	@echo "=====> Installing golangci-lint..."
	cd tools && $(GOCMD) install github.com/golangci/golangci-lint/cmd/golangci-lint

## mod-tidy: Tidy Go modules
mod-tidy:
	$(GOCMD) mod tidy  -compat=1.19
	cd tools && $(GOCMD) mod tidy -compat=1.19

## tidy: Tidy modules and format the code
tidy: mod-tidy format

## out: Make a directory for output
out:
	@mkdir out || (exit 0)
	mkdir out$/tests || (exit 0)

# Display help information
help: Makefile
	@echo ""
	@echo "Usage:"
	@echo ""
	@echo "  make [target]"
	@echo ""
	@echo "Targets:"
	@echo ""
	@awk -F ':|##' '/^[^\.%\t][^\t]*:.*##/{printf "  \033[36m%-20s\033[0m %s\n", $$1, $$NF}' $(MAKEFILE_LIST) | sort
	@sed -n 's/^##//p' ${MAKEFILE_LIST} | column -t -s ':' |  sed -e 's/^/ /'