GO ?= go
NPM ?= npm
NPM_BIN ?= $(shell command -v "$(NPM)" 2>/dev/null)
.DEFAULT_GOAL := build
APP_NAME ?= GitGit
INSTALL_DIR ?= $(HOME)/Applications
PRODUCT_VERSION ?= 0.2.0
MACOS_MINIMUM_VERSION ?= 11.0
BUILD_TIMESTAMP ?= $(shell date '+%Y%m%d%H%M%S')
BUILD_TIMESTAMP := $(BUILD_TIMESTAMP)
BUILD_VERSION ?= $(shell printf '%s' "$(BUILD_TIMESTAMP)" | sed -E 's/^([0-9]{4})([0-9]{2})([0-9]{2})([0-9]{2})([0-9]{2})([0-9]{2})$$/\1.\2.\3.\4\5\6/')
BUILD_VERSION := $(BUILD_VERSION)
SOURCE_REVISION ?= $(shell git describe --always --dirty 2>/dev/null || printf 'unknown')
SOURCE_REVISION := $(SOURCE_REVISION)
TEST_SEED ?= 20260718
SUBGIT_DIR ?= $(CURDIR)/subgit
SUBGIT_SETUP := testdata/subgit/setup.sh
DESKTOP_DIR := $(CURDIR)/desktop
DESKTOP_FRONTEND := $(DESKTOP_DIR)/frontend
BUILD_BIN_DIR := $(DESKTOP_DIR)/build/bin
APP_BUNDLE := $(BUILD_BIN_DIR)/$(APP_NAME).app
INSTALLED_APP := $(INSTALL_DIR)/$(APP_NAME).app
INSTALL_STAGING_APP := $(INSTALL_DIR)/.$(APP_NAME).app.installing
WAILS_PACKAGE ?= github.com/wailsapp/wails/v2/cmd/wails@v2.13.0
WAILS_DEVSERVER ?= localhost:34116
CACHE_DIR ?= $(HOME)/Library/Caches/com.wails.gitgit
HOST_OS ?= $(shell uname -s)
HOST_ARCH ?= $(shell uname -m)

ifneq ($(HOST_OS),Darwin)
$(error GitGit Make targets require macOS; detected $(HOST_OS))
endif

.PHONY: check-npm deps frontend-build build bundle install clean uninstall dev dev-browser test test-random check check-native subgit subgit-reset

check-npm:
	@test -n "$(NPM_BIN)" || { echo "npm was not found. Set NPM=/absolute/path/to/npm or add npm to PATH." >&2; exit 1; }

deps: check-npm
	"$(NPM_BIN)" --prefix "$(DESKTOP_FRONTEND)" ci

frontend-build: deps
	"$(NPM_BIN)" --prefix "$(DESKTOP_FRONTEND)" run build

build:
	@set -eu; \
	cleanup_build() { \
		status=$$?; \
		trap - EXIT HUP INT TERM; \
		test -n "$(BUILD_BIN_DIR)" && test "$(BUILD_BIN_DIR)" != "/" && rm -rf "$(BUILD_BIN_DIR)"; \
		exit "$$status"; \
	}; \
	trap cleanup_build EXIT HUP INT TERM; \
	$(MAKE) --no-print-directory bundle NPM_BIN="$(NPM_BIN)" BUILD_TIMESTAMP="$(BUILD_TIMESTAMP)" BUILD_VERSION="$(BUILD_VERSION)" SOURCE_REVISION="$(SOURCE_REVISION)"

bundle:
	@set -eu; \
	bundle_complete=0; \
	cleanup_incomplete_bundle() { \
		status=$$?; \
		trap - EXIT HUP INT TERM; \
		if test "$$bundle_complete" -ne 1; then \
			test -n "$(BUILD_BIN_DIR)" && test "$(BUILD_BIN_DIR)" != "/" && rm -rf "$(BUILD_BIN_DIR)"; \
		fi; \
		exit "$$status"; \
	}; \
	trap cleanup_incomplete_bundle EXIT HUP INT TERM; \
	test -n "$(BUILD_BIN_DIR)" && test "$(BUILD_BIN_DIR)" != "/"; \
	rm -rf "$(BUILD_BIN_DIR)"; \
	$(MAKE) --no-print-directory frontend-build NPM_BIN="$(NPM_BIN)"; \
	PATH="$(dir $(NPM_BIN)):$(PATH)" MACOSX_DEPLOYMENT_TARGET="$(MACOS_MINIMUM_VERSION)" $(GO) -C "$(DESKTOP_DIR)" run "$(WAILS_PACKAGE)" build -s -ldflags "-X main.productVersion=$(PRODUCT_VERSION) -X main.buildVersion=$(BUILD_VERSION) -X main.sourceRevision=$(SOURCE_REVISION)"; \
	test -d "$(APP_BUNDLE)"; \
	/usr/libexec/PlistBuddy -c "Set :CFBundleShortVersionString $(PRODUCT_VERSION)" "$(APP_BUNDLE)/Contents/Info.plist"; \
	/usr/libexec/PlistBuddy -c "Set :CFBundleVersion $(BUILD_TIMESTAMP)" "$(APP_BUNDLE)/Contents/Info.plist"; \
	/usr/libexec/PlistBuddy -c "Set :GitGitBuildVersion $(BUILD_VERSION)" "$(APP_BUNDLE)/Contents/Info.plist"; \
	/usr/libexec/PlistBuddy -c "Set :GitGitSourceRevision $(SOURCE_REVISION)" "$(APP_BUNDLE)/Contents/Info.plist"; \
	/usr/libexec/PlistBuddy -c "Set :LSMinimumSystemVersion $(MACOS_MINIMUM_VERSION)" "$(APP_BUNDLE)/Contents/Info.plist"; \
	codesign --force --deep --sign - "$(APP_BUNDLE)"; \
	bundle_complete=1

install:
	@set -eu; \
	test -n "$(APP_NAME)"; \
	test -n "$(INSTALL_DIR)" && test "$(INSTALL_DIR)" != "/"; \
	cleanup_install() { \
		status=$$?; \
		trap - EXIT HUP INT TERM; \
		test -n "$(BUILD_BIN_DIR)" && test "$(BUILD_BIN_DIR)" != "/" && rm -rf "$(BUILD_BIN_DIR)"; \
		rm -rf "$(INSTALL_STAGING_APP)"; \
		exit "$$status"; \
	}; \
	trap cleanup_install EXIT HUP INT TERM; \
	$(MAKE) --no-print-directory bundle NPM_BIN="$(NPM_BIN)" BUILD_TIMESTAMP="$(BUILD_TIMESTAMP)" BUILD_VERSION="$(BUILD_VERSION)" SOURCE_REVISION="$(SOURCE_REVISION)"; \
	test -d "$(APP_BUNDLE)"; \
	mkdir -p "$(INSTALL_DIR)"; \
	rm -rf "$(INSTALL_STAGING_APP)"; \
	ditto "$(APP_BUNDLE)" "$(INSTALL_STAGING_APP)"; \
	rm -rf "$(INSTALLED_APP)"; \
	mv "$(INSTALL_STAGING_APP)" "$(INSTALLED_APP)"

clean:
	test -n "$(BUILD_BIN_DIR)" && test "$(BUILD_BIN_DIR)" != "/"
	rm -rf "$(BUILD_BIN_DIR)"

uninstall: clean
	test -n "$(APP_NAME)"
	test -n "$(INSTALL_DIR)" && test "$(INSTALL_DIR)" != "/"
	test -n "$(CACHE_DIR)" && test "$(CACHE_DIR)" != "/"
	rm -rf "$(INSTALLED_APP)"
	rm -rf "$(CACHE_DIR)"

dev: dev-browser

dev-browser:
	@set -eu; \
	cleanup_dev() { \
		status=$$?; \
		trap - EXIT HUP INT TERM; \
		test -n "$(BUILD_BIN_DIR)" && test "$(BUILD_BIN_DIR)" != "/" && rm -rf "$(BUILD_BIN_DIR)"; \
		exit "$$status"; \
	}; \
	trap cleanup_dev EXIT HUP INT TERM; \
	$(MAKE) --no-print-directory check-npm NPM_BIN="$(NPM_BIN)"; \
	PATH="$(dir $(NPM_BIN)):$(PATH)" GITGIT_BROWSER_DEV=1 $(GO) -C "$(DESKTOP_DIR)" run "$(WAILS_PACKAGE)" dev -devserver "$(WAILS_DEVSERVER)"

test: check-npm
	"$(NPM_BIN)" --prefix "$(DESKTOP_FRONTEND)" test
	$(GO) test -race -count=1 ./...
	$(GO) -C "$(DESKTOP_DIR)" test -count=1 ./...

test-random:
	RANDOM_TEST_SEED="$(TEST_SEED)" $(GO) test -count=1 -shuffle="$(TEST_SEED)" ./...

check:
	@set -eu; \
	cleanup_check_suite() { \
		status=$$?; \
		trap - EXIT HUP INT TERM; \
		test -n "$(BUILD_BIN_DIR)" && test "$(BUILD_BIN_DIR)" != "/" && rm -rf "$(BUILD_BIN_DIR)"; \
		exit "$$status"; \
	}; \
	trap cleanup_check_suite EXIT HUP INT TERM; \
	$(MAKE) --no-print-directory deps NPM_BIN="$(NPM_BIN)"; \
	"$(NPM_BIN)" --prefix "$(DESKTOP_FRONTEND)" test; \
	"$(NPM_BIN)" --prefix "$(DESKTOP_FRONTEND)" run check; \
	"$(NPM_BIN)" --prefix "$(DESKTOP_FRONTEND)" run build; \
	$(GO) test -race -count=1 ./...; \
	$(GO) vet ./...; \
	$(GO) -C "$(DESKTOP_DIR)" test -count=1 ./...; \
	$(GO) -C "$(DESKTOP_DIR)" vet ./...; \
	$(MAKE) --no-print-directory check-native NPM_BIN="$(NPM_BIN)" PRODUCT_VERSION="$(PRODUCT_VERSION)" BUILD_TIMESTAMP="$(BUILD_TIMESTAMP)" BUILD_VERSION="$(BUILD_VERSION)" SOURCE_REVISION="$(SOURCE_REVISION)"

check-native:
	@set -eu; \
	cleanup_check() { \
		status=$$?; \
		trap - EXIT HUP INT TERM; \
		test -n "$(BUILD_BIN_DIR)" && test "$(BUILD_BIN_DIR)" != "/" && rm -rf "$(BUILD_BIN_DIR)"; \
		exit "$$status"; \
	}; \
	trap cleanup_check EXIT HUP INT TERM; \
	$(MAKE) --no-print-directory bundle NPM_BIN="$(NPM_BIN)" BUILD_TIMESTAMP="$(BUILD_TIMESTAMP)" BUILD_VERSION="$(BUILD_VERSION)" SOURCE_REVISION="$(SOURCE_REVISION)"; \
	test "$$(/usr/libexec/PlistBuddy -c 'Print :CFBundleShortVersionString' "$(APP_BUNDLE)/Contents/Info.plist")" = "$(PRODUCT_VERSION)"; \
	test "$$(/usr/libexec/PlistBuddy -c 'Print :CFBundleVersion' "$(APP_BUNDLE)/Contents/Info.plist")" = "$(BUILD_TIMESTAMP)"; \
	test "$$(/usr/libexec/PlistBuddy -c 'Print :GitGitBuildVersion' "$(APP_BUNDLE)/Contents/Info.plist")" = "$(BUILD_VERSION)"; \
	test "$$(/usr/libexec/PlistBuddy -c 'Print :GitGitSourceRevision' "$(APP_BUNDLE)/Contents/Info.plist")" = "$(SOURCE_REVISION)"; \
	test "$$(/usr/libexec/PlistBuddy -c 'Print :LSMinimumSystemVersion' "$(APP_BUNDLE)/Contents/Info.plist")" = "$(MACOS_MINIMUM_VERSION)"; \
	file "$(APP_BUNDLE)/Contents/MacOS/$(APP_NAME)" | grep -q '$(HOST_ARCH)'; \
	vtool -show-build "$(APP_BUNDLE)/Contents/MacOS/$(APP_NAME)" | grep -q 'minos $(MACOS_MINIMUM_VERSION)'; \
	codesign --verify --deep --strict --verbose=2 "$(APP_BUNDLE)"

subgit:
	sh "$(SUBGIT_SETUP)" create "$(SUBGIT_DIR)"

subgit-reset:
	sh "$(SUBGIT_SETUP)" reset "$(SUBGIT_DIR)"
