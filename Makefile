.PHONY: install dev-install build build-kpixiv build-kpixivctl test lint clean uninstall dist

BIN_DIR        = $(PWD)/bin
DIST_DIR       = $(PWD)/dist
KPIXIV_BIN     = $(HOME)/.local/bin/kpixiv
KPIXIVCTL_BIN  = $(HOME)/.local/bin/kpixivctl
APPS_DIR       = $(HOME)/.local/share/applications
ICONS_DIR      = $(HOME)/.local/share/icons/hicolor/scalable/apps
QT6_BIN        = /usr/lib/qt6/bin

VERSION      ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
PKG          = github.com/alphonse927/kpixiv/internal/build
LDFLAGS      = -ldflags "-X $(PKG).Version=$(VERSION)"
DIST_LDFLAGS = -ldflags "-X $(PKG).Version=$(VERSION) -s -w"

build-kpixiv:
	go build $(LDFLAGS) -o $(BIN_DIR)/kpixiv ./cmd/kpixiv

build-kpixivctl:
	go build $(LDFLAGS) -o $(BIN_DIR)/kpixivctl ./cmd/kpixivctl

build: build-kpixiv build-kpixivctl

install-desktop:
	mkdir -p $(APPS_DIR) $(ICONS_DIR)
	sed "s|@HOME@|$(HOME)|g" configs/kpixiv.desktop > $(APPS_DIR)/kpixiv.desktop
	cp internal/tray/assets/kpixiv-symbolic.svg $(ICONS_DIR)/kpixiv.svg

register-scheme: install-desktop
	PATH="$(PATH):$(QT6_BIN)" xdg-mime default kpixiv.desktop x-scheme-handler/pixiv
	PATH="$(PATH):$(QT6_BIN)" xdg-desktop-menu forceupdate

install: build install-desktop
	rm -f $(KPIXIV_BIN) $(KPIXIVCTL_BIN)
	cp $(BIN_DIR)/kpixiv $(KPIXIV_BIN)
	cp $(BIN_DIR)/kpixivctl $(KPIXIVCTL_BIN)
	PATH="$(PATH):$(QT6_BIN)" xdg-mime default kpixiv.desktop x-scheme-handler/pixiv
	PATH="$(PATH):$(QT6_BIN)" xdg-desktop-menu forceupdate

dev-install: build install-desktop
	ln -sf $(BIN_DIR)/kpixiv $(KPIXIV_BIN)
	ln -sf $(BIN_DIR)/kpixivctl $(KPIXIVCTL_BIN)
	PATH="$(PATH):$(QT6_BIN)" xdg-mime default kpixiv.desktop x-scheme-handler/pixiv
	PATH="$(PATH):$(QT6_BIN)" xdg-desktop-menu forceupdate

uninstall:
	rm -f $(KPIXIV_BIN) $(KPIXIVCTL_BIN)
	rm -f $(APPS_DIR)/kpixiv.desktop
	-xdg-mime default "" x-scheme-handler/pixiv 2>/dev/null
	rm -f $(ICONS_DIR)/kpixiv.svg
	rm -f $(HOME)/.config/systemd/user/kpixiv.service
	-systemctl --user daemon-reload 2>/dev/null

test:
	go test ./...

vet:
	go vet ./...

lint:
	golangci-lint run

clean:
	rm -rf $(BIN_DIR) $(DIST_DIR)

dist:
	rm -rf $(DIST_DIR)
	mkdir -p $(DIST_DIR)/kpixiv-$(VERSION)-linux-amd64
	go build $(DIST_LDFLAGS) -o $(DIST_DIR)/kpixiv-$(VERSION)-linux-amd64/kpixiv ./cmd/kpixiv
	go build $(DIST_LDFLAGS) -o $(DIST_DIR)/kpixiv-$(VERSION)-linux-amd64/kpixivctl ./cmd/kpixivctl
	cd $(DIST_DIR) && tar czf kpixiv-$(VERSION)-linux-amd64.tar.gz kpixiv-$(VERSION)-linux-amd64
	@echo "Created $(DIST_DIR)/kpixiv-$(VERSION)-linux-amd64.tar.gz"
