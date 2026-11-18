BINARY := vidpuller
GO     ?= go
VERSION := $(shell git describe --tags --always 2>/dev/null || echo dev)
LDFLAGS := -s -w -X main.version=$(VERSION)

.PHONY: all build test vet fmt lang check install clean

all: check build

build:
	CGO_ENABLED=0 $(GO) build -trimpath -ldflags "$(LDFLAGS)" -o $(BINARY) .

test:
	$(GO) test ./...

vet:
	$(GO) vet ./...

fmt:
	@out=$$(gofmt -l .); if [ -n "$$out" ]; then echo "gofmt needed:"; echo "$$out"; exit 1; fi

lang:
	$(GO) run ./tools/langcheck

check: fmt vet test lang

install: build
	mkdir -p $(HOME)/.local/bin
	install -m 0755 $(BINARY) $(HOME)/.local/bin/$(BINARY)

clean:
	rm -f $(BINARY)
