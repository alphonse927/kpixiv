.PHONY: install

BINARY_NAME=kpixiv
INSTALL_PATH="$(HOME)/go/bin"

install:
	go build -o bin/$(BINARY_NAME) ./cmd/kpixiv
	cp bin/$(BINARY_NAME) $(INSTALL_PATH)/$(BINARY_NAME)
	chmod +x $(INSTALL_PATH)/$(BINARY_NAME)