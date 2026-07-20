GO ?= go
NPM ?= npm
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

.PHONY: deps build bundle install clean uninstall dev dev-browser test test-random check check-native subgit subgit-reset

deps:
	$(NPM) --prefix "$(DESKTOP_FRONTEND)" ci

build: bundle

bundle:
	MACOSX_DEPLOYMENT_TARGET="$(MACOS_MINIMUM_VERSION)" $(GO) -C "$(DESKTOP_DIR)" run "$(WAILS_PACKAGE)" build -platform darwin/arm64 -ldflags "-X main.productVersion=$(PRODUCT_VERSION) -X main.buildVersion=$(BUILD_VERSION) -X main.sourceRevision=$(SOURCE_REVISION)"
	test -d "$(APP_BUNDLE)"
	/usr/libexec/PlistBuddy -c "Set :CFBundleShortVersionString $(PRODUCT_VERSION)" "$(APP_BUNDLE)/Contents/Info.plist"
	/usr/libexec/PlistBuddy -c "Set :CFBundleVersion $(BUILD_TIMESTAMP)" "$(APP_BUNDLE)/Contents/Info.plist"
	/usr/libexec/PlistBuddy -c "Set :GitGitBuildVersion $(BUILD_VERSION)" "$(APP_BUNDLE)/Contents/Info.plist"
	/usr/libexec/PlistBuddy -c "Set :GitGitSourceRevision $(SOURCE_REVISION)" "$(APP_BUNDLE)/Contents/Info.plist"
	/usr/libexec/PlistBuddy -c "Set :LSMinimumSystemVersion $(MACOS_MINIMUM_VERSION)" "$(APP_BUNDLE)/Contents/Info.plist"
	codesign --force --deep --sign - "$(APP_BUNDLE)"

install: bundle
	test -n "$(APP_NAME)"
	test -n "$(INSTALL_DIR)" && test "$(INSTALL_DIR)" != "/"
	test -d "$(APP_BUNDLE)"
	mkdir -p "$(INSTALL_DIR)"
	rm -rf "$(INSTALL_STAGING_APP)"
	ditto "$(APP_BUNDLE)" "$(INSTALL_STAGING_APP)"
	rm -rf "$(INSTALLED_APP)"
	mv "$(INSTALL_STAGING_APP)" "$(INSTALLED_APP)"

clean:
	test -n "$(BUILD_BIN_DIR)" && test "$(BUILD_BIN_DIR)" != "/"
	rm -rf "$(BUILD_BIN_DIR)"

uninstall:
	test -n "$(APP_NAME)"
	test -n "$(INSTALL_DIR)" && test "$(INSTALL_DIR)" != "/"
	test -n "$(CACHE_DIR)" && test "$(CACHE_DIR)" != "/"
	rm -rf "$(INSTALLED_APP)"
	rm -rf "$(CACHE_DIR)"

dev: dev-browser

dev-browser:
	$(GO) -C "$(DESKTOP_DIR)" run "$(WAILS_PACKAGE)" dev -devserver "$(WAILS_DEVSERVER)"

test:
	$(NPM) --prefix "$(DESKTOP_FRONTEND)" test
	$(GO) test -race -count=1 ./...
	$(GO) -C "$(DESKTOP_DIR)" test -count=1 ./...

test-random:
	RANDOM_TEST_SEED="$(TEST_SEED)" $(GO) test -count=1 -shuffle="$(TEST_SEED)" ./...

check: deps
	$(NPM) --prefix "$(DESKTOP_FRONTEND)" test
	$(NPM) --prefix "$(DESKTOP_FRONTEND)" run check
	$(NPM) --prefix "$(DESKTOP_FRONTEND)" run build
	$(GO) test -race -count=1 ./...
	$(GO) vet ./...
	$(GO) -C "$(DESKTOP_DIR)" test -count=1 ./...
	$(GO) -C "$(DESKTOP_DIR)" vet ./...
	$(MAKE) check-native PRODUCT_VERSION="$(PRODUCT_VERSION)" BUILD_TIMESTAMP="$(BUILD_TIMESTAMP)" BUILD_VERSION="$(BUILD_VERSION)" SOURCE_REVISION="$(SOURCE_REVISION)"

check-native: bundle
	test "$$(/usr/libexec/PlistBuddy -c 'Print :CFBundleShortVersionString' "$(APP_BUNDLE)/Contents/Info.plist")" = "$(PRODUCT_VERSION)"
	test "$$(/usr/libexec/PlistBuddy -c 'Print :CFBundleVersion' "$(APP_BUNDLE)/Contents/Info.plist")" = "$(BUILD_TIMESTAMP)"
	test "$$(/usr/libexec/PlistBuddy -c 'Print :GitGitBuildVersion' "$(APP_BUNDLE)/Contents/Info.plist")" = "$(BUILD_VERSION)"
	test "$$(/usr/libexec/PlistBuddy -c 'Print :GitGitSourceRevision' "$(APP_BUNDLE)/Contents/Info.plist")" = "$(SOURCE_REVISION)"
	test "$$(/usr/libexec/PlistBuddy -c 'Print :LSMinimumSystemVersion' "$(APP_BUNDLE)/Contents/Info.plist")" = "$(MACOS_MINIMUM_VERSION)"
	file "$(APP_BUNDLE)/Contents/MacOS/$(APP_NAME)" | grep -q 'arm64'
	vtool -show-build "$(APP_BUNDLE)/Contents/MacOS/$(APP_NAME)" | grep -q 'minos $(MACOS_MINIMUM_VERSION)'
	codesign --verify --deep --strict --verbose=2 "$(APP_BUNDLE)"

subgit:
	sh "$(SUBGIT_SETUP)" create "$(SUBGIT_DIR)"

subgit-reset:
	sh "$(SUBGIT_SETUP)" reset "$(SUBGIT_DIR)"
