GOCACHE ?= $(CURDIR)/.cache
GOMODCACHE ?= $(CURDIR)/.modcache
GOPATH ?= $(CURDIR)/.gopath

.PHONY: tidy build test sandbox tag

tidy:
	go mod tidy

build:
	GOCACHE=$(GOCACHE) GOMODCACHE=$(GOMODCACHE) GOPATH=$(GOPATH) go build ./...

test:
	GOCACHE=$(GOCACHE) GOMODCACHE=$(GOMODCACHE) GOPATH=$(GOPATH) go test ./...

sandbox:
	go run ./cmd/ratio1-sandbox

tag:
ifndef VERSION
	$(error VERSION is not set. Usage: make tag VERSION=v0.1.0)
endif
	git tag $(VERSION)
