# Go parameters
GOCMD?=go
GOTEST=$(GOCMD) test -mod=vendor

all: test

imports:
	$(GOCMD) install -mod=vendor golang.org/x/tools/cmd/goimports
	test -z $$(goimports -l -w -local github.com/buildpack/imgutil $$(find . -type f -name '*.go' -not -path "./vendor/*"))

format:
	test -z $$($(GOCMD) fmt ./...)

vet:
	$(GOCMD) vet $$($(GOCMD) list ./... | grep -v /testdata/)

test: format imports vet
	$(GOTEST) -parallel=1 -count=1 -v ./...
