.PHONY: build install clean test

BINARY_NAME=kpixiv
INSTALL_PATH="$(HOME)/go/bin"
GO=go
GOFLAGS=-ldflags="-s -w"

build:
	$(GO) build $(GOFLAGS) -o bin/$(BINARY_NAME) ./cmd/kpixiv

install: build
	cp bin/$(BINARY_NAME) $(INSTALL_PATH)/$(BINARY_NAME)
	chmod +x $(INSTALL_PATH)/$(BINARY_NAME)

clean:
	rm -f $(BINARY_NAME)

test:
	$(GO) test -v ./...

run:
	$(GO) run ./cmd/kpixiv

dev:
	$(GO) build -o $(BINARY_NAME) ./cmd/kpixiv && ./$(BINARY_NAME)

deps:
	$(GO) mod download
	$(GO) mod tidy

lint:
	golangci-lint run

fmt:
	$(GO) fmt ./...

vet:
	$(GO) vet ./...
