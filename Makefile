.PHONY: install

BIN = $(HOME)/.local/bin/kpixiv
TRAY_BIN = $(HOME)/.local/bin/kpixiv-tray
SYSTEMD_DIR = $(HOME)/.config/systemd/user

install:
	go build -o bin/kpixiv ./cmd/kpixiv
	go build -o bin/kpixiv-tray ./cmd/kpixiv-tray
	ln -sf $(PWD)/bin/kpixiv $(BIN)
	ln -sf $(PWD)/bin/kpixiv-tray $(TRAY_BIN)
	mkdir -p $(SYSTEMD_DIR)
	cp configs/kpixiv.service $(SYSTEMD_DIR)/kpixiv.service
	systemctl --user daemon-reload
