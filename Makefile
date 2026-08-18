.PHONY: help build build-kpixiv build-kpixivctl clean \
        install dev-install uninstall dist install-desktop install-service register-scheme

Q       ?= @
step     = $(Q)printf '==> %s\n' '$(1)'

BIN_DIR        = $(PWD)/bin
DIST_DIR       = $(PWD)/dist
KPIXIV_BIN     = $(HOME)/.local/bin/kpixiv
KPIXIVCTL_BIN  = $(HOME)/.local/bin/kpixivctl

XDG_DATA_HOME  ?= $(HOME)/.local/share
APPLICATIONS_DIR = $(XDG_DATA_HOME)/applications
ICONS_DIR      = $(XDG_DATA_HOME)/icons/hicolor/scalable/apps
SYSTEMD_UNIT_DIR = $(HOME)/.config/systemd/user

QT6_BIN        = /usr/lib/qt6/bin
XDG_ENV        = PATH="$(PATH):$(QT6_BIN)"

VERSION       ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
COMMIT        ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
DATE          ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
PKG            = github.com/alphonse927/kpixiv/internal/build
LDFLAGS        = -ldflags "-X $(PKG).Version=$(VERSION) -X $(PKG).Commit=$(COMMIT) -X $(PKG).Date=$(DATE)"
DIST_LDFLAGS   = -ldflags "-X $(PKG).Version=$(VERSION) -X $(PKG).Commit=$(COMMIT) -X $(PKG).Date=$(DATE) -s -w"

help:
	@echo 'Usage: make <target>'
	@echo ''
	@echo 'Build:'
	@echo '  build         Build both binaries'
	@echo '  dist          Create release tarball'
	@echo ''
	@echo 'Install:'
	@echo '  install       Build, install to ~/.local, and start the systemd user service'
	@echo '  dev-install   Build, symlink for development, and start the systemd user service'
	@echo '  uninstall     Remove all installed files and stop the systemd user service'
	@echo ''
	@echo 'Housekeeping:'
	@echo '  clean         Remove build and dist directories'

build-kpixiv:
	$(call step,Building kpixiv)
	$(Q)go build $(LDFLAGS) -o $(BIN_DIR)/kpixiv ./cmd/kpixiv

build-kpixivctl:
	$(call step,Building kpixivctl)
	$(Q)go build $(LDFLAGS) -o $(BIN_DIR)/kpixivctl ./cmd/kpixivctl

build: build-kpixiv build-kpixivctl

install-desktop:
	$(call step,Installing desktop entry)
	$(Q)mkdir -p $(APPLICATIONS_DIR) $(ICONS_DIR)
	$(Q)sed "s|@HOME@|$(HOME)|g" configs/kpixiv.desktop > $(APPLICATIONS_DIR)/kpixiv.desktop
	$(Q)cp internal/tray/assets/kpixiv-symbolic.svg $(ICONS_DIR)/kpixiv.svg

install-service:
	$(call step,Installing systemd user service)
	$(Q)mkdir -p $(SYSTEMD_UNIT_DIR)
	$(Q)cp internal/platform/kpixiv.service $(SYSTEMD_UNIT_DIR)/kpixiv.service
	$(Q)systemctl --user daemon-reload
	$(Q)systemctl --user enable --now kpixiv.service
	$(call step,kPixiv is running as a systemd user service)

register-scheme: install-desktop
	$(call step,Registering pixiv:// handler)
	$(Q)$(XDG_ENV) xdg-mime default kpixiv.desktop x-scheme-handler/pixiv
	$(Q)$(XDG_ENV) xdg-desktop-menu forceupdate

install: build register-scheme
	$(call step,Installing binaries)
	$(Q)rm -f $(KPIXIV_BIN) $(KPIXIVCTL_BIN)
	$(Q)cp $(BIN_DIR)/kpixiv $(KPIXIV_BIN)
	$(Q)cp $(BIN_DIR)/kpixivctl $(KPIXIVCTL_BIN)
	$(Q)$(MAKE) install-service
	$(call step,Installation complete)

dev-install: build register-scheme
	$(call step,Installing development symlinks)
	$(Q)ln -sf $(BIN_DIR)/kpixiv $(KPIXIV_BIN)
	$(Q)ln -sf $(BIN_DIR)/kpixivctl $(KPIXIVCTL_BIN)
	$(Q)$(MAKE) install-service
	$(call step,Installation complete)

uninstall:
	$(call step,Removing installed files)
	$(Q)rm -f $(KPIXIV_BIN) $(KPIXIVCTL_BIN)
	$(Q)rm -f $(APPLICATIONS_DIR)/kpixiv.desktop
	$(Q)-$(XDG_ENV) xdg-mime default "" x-scheme-handler/pixiv 2>/dev/null
	$(Q)rm -f $(ICONS_DIR)/kpixiv.svg
	$(Q)-systemctl --user disable --now kpixiv.service 2>/dev/null
	$(Q)rm -f $(SYSTEMD_UNIT_DIR)/kpixiv.service
	$(Q)-systemctl --user daemon-reload 2>/dev/null
	$(call step,Uninstall complete)

clean:
	$(call step,Cleaning build artifacts)
	$(Q)rm -rf $(BIN_DIR) $(DIST_DIR)

dist:
	$(call step,Creating release tarball)
	$(Q)rm -rf $(DIST_DIR)
	$(Q)mkdir -p $(DIST_DIR)/kpixiv-$(VERSION)-linux-amd64
	$(Q)go build $(DIST_LDFLAGS) -o $(DIST_DIR)/kpixiv-$(VERSION)-linux-amd64/kpixiv ./cmd/kpixiv
	$(Q)go build $(DIST_LDFLAGS) -o $(DIST_DIR)/kpixiv-$(VERSION)-linux-amd64/kpixivctl ./cmd/kpixivctl
	$(Q)cd $(DIST_DIR) && tar czf kpixiv-$(VERSION)-linux-amd64.tar.gz kpixiv-$(VERSION)-linux-amd64
	$(call step,Created $(DIST_DIR)/kpixiv-$(VERSION)-linux-amd64.tar.gz)
