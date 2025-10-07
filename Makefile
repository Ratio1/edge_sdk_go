GOCACHE ?= $(CURDIR)/.cache
GOMODCACHE ?= $(CURDIR)/.modcache
GOPATH ?= $(CURDIR)/.gopath
DIST_DIR ?= $(CURDIR)/dist
GOOS ?= $(shell go env GOOS)
GOARCH ?= $(shell go env GOARCH)
SANDBOX_BIN := ratio1-sandbox
SANDBOX_BIN_NAME := $(SANDBOX_BIN)$(if $(filter windows,$(GOOS)),.exe,)
SANDBOX_ARCHIVE := $(DIST_DIR)/$(SANDBOX_BIN)_$(GOOS)_$(GOARCH).tar.gz

.PHONY: tidy build test sandbox sandbox-dist clean-dist tag

tidy:
	go mod tidy

build:
	GOCACHE=$(GOCACHE) GOMODCACHE=$(GOMODCACHE) GOPATH=$(GOPATH) go build ./...

test:
	GOCACHE=$(GOCACHE) GOMODCACHE=$(GOMODCACHE) GOPATH=$(GOPATH) go test ./...

sandbox:
	go run ./cmd/ratio1-sandbox

$(DIST_DIR):
	mkdir -p $@

clean-dist:
	rm -rf $(DIST_DIR)

sandbox-dist: clean-dist | $(DIST_DIR)
	GOOS=$(GOOS) GOARCH=$(GOARCH) CGO_ENABLED=0 \
		GOCACHE=$(GOCACHE) GOMODCACHE=$(GOMODCACHE) GOPATH=$(GOPATH) \
		go build -ldflags '-s -w' -o $(DIST_DIR)/$(SANDBOX_BIN_NAME) ./cmd/ratio1-sandbox
	cd $(DIST_DIR) && tar -czf $(SANDBOX_BIN)_$(GOOS)_$(GOARCH).tar.gz $(SANDBOX_BIN_NAME)
	@printf 'sandbox archive ready: %s\n' $(SANDBOX_ARCHIVE)

tag:
ifndef VERSION
	$(error VERSION is not set. Usage: make tag VERSION=v0.1.0)
endif
	git tag $(VERSION)
