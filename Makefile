.PHONY: install build build-kpixiv build-kpixivctl

BIN_DIR = $(PWD)/bin
KPIXIV_BIN = $(HOME)/.local/bin/kpixiv
KPIXIVCTL_BIN = $(HOME)/.local/bin/kpixivctl
SYSTEMD_DIR = $(HOME)/.config/systemd/user

build-kpixiv:
	go build -o $(BIN_DIR)/kpixiv ./cmd/kpixiv

build-kpixivctl:
	go build -o $(BIN_DIR)/kpixivctl ./cmd/kpixivctl

build: build-kpixiv build-kpixivctl

install: build
	ln -sf $(BIN_DIR)/kpixiv $(KPIXIV_BIN)
	ln -sf $(BIN_DIR)/kpixivctl $(KPIXIVCTL_BIN)
	mkdir -p $(SYSTEMD_DIR)
	cp configs/kpixiv.service $(SYSTEMD_DIR)/kpixiv.service
	systemctl --user daemon-reload
