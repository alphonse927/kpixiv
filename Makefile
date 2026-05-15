.PHONY: install

BIN = $(HOME)/.local/bin/kpixiv
SYSTEMD_DIR = $(HOME)/.config/systemd/user

install:
	go build -o bin/kpixiv ./cmd/kpixiv
	ln -sf $(PWD)/bin/kpixiv $(BIN)
	mkdir -p $(SYSTEMD_DIR)
	cp configs/kpixiv.service $(SYSTEMD_DIR)/kpixiv.service
	systemctl --user daemon-reload