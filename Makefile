VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
REPO    := lcajigasm/omega
LDFLAGS := -s -w -X main.version=$(VERSION)

.PHONY: build install test lint clean formula cross capture gui gui-app gui-dmg

build:
	go build -ldflags "$(LDFLAGS)" -o bin/omega ./cmd/omega

install:
	go install -ldflags "$(LDFLAGS)" ./cmd/omega

test:
	go test ./...

lint:
	gofmt -l . | (! grep .) && go vet ./...

clean:
	rm -rf bin

# The desktop app. It is built apart from the command-line tool: the window
# uses the system webview, which needs cgo on macOS and Linux (Xcode command
# line tools; on Linux libgtk-4-dev and libwebkitgtk-6.0-dev). Windows needs
# no cgo and hides the console window.
GUI_TAGS    := gui,production
GUI_LDFLAGS := $(LDFLAGS)
GUI_EXT     :=
ifeq ($(OS),Windows_NT)
GUI_LDFLAGS += -H windowsgui
GUI_EXT     := .exe
endif
ifeq ($(shell uname -s 2>/dev/null),Darwin)
export MACOSX_DEPLOYMENT_TARGET := 13.0
export CGO_CFLAGS  := -mmacosx-version-min=13.0
export CGO_LDFLAGS := -mmacosx-version-min=13.0
endif

gui:
	go build -trimpath -tags "$(GUI_TAGS)" -ldflags "$(GUI_LDFLAGS)" -o bin/omega-gui$(GUI_EXT) ./cmd/omega-gui

# macOS only: wrap the binary in bin/Omega.app so it opens from Finder with
# its own Dock icon. Ad-hoc signed, for this machine's architecture.
gui-app: gui
	@rm -rf bin/Omega.app && mkdir -p bin/Omega.app/Contents/MacOS bin/Omega.app/Contents/Resources
	@cp bin/omega-gui bin/Omega.app/Contents/MacOS/omega-gui
	@cp cmd/omega-gui/omega.icns bin/Omega.app/Contents/Resources/omega.icns
	@sed "s/@VERSION@/$(patsubst v%,%,$(VERSION))/g" cmd/omega-gui/Info.plist > bin/Omega.app/Contents/Info.plist
	@codesign --force --sign - bin/Omega.app
	@echo bin/Omega.app

# macOS only: the universal app in a disk image, as a release publishes it.
gui-dmg:
	scripts/package_macos.sh "$(VERSION)" bin

# Local cross-compile of the release targets (no archives).
cross:
	@mkdir -p bin
	@for pair in linux/amd64 linux/arm64 darwin/amd64 darwin/arm64 windows/amd64 windows/arm64; do \
		os=$${pair%/*}; arch=$${pair#*/}; ext=; \
		if [ "$$os" = windows ]; then ext=.exe; fi; \
		echo "$$os/$$arch"; \
		GOOS=$$os GOARCH=$$arch CGO_ENABLED=0 go build -trimpath -ldflags "$(LDFLAGS)" -o bin/omega-$$os-$$arch$$ext ./cmd/omega || exit 1; \
	done

# Record README and Pages captures from the binary built in this checkout.
capture:
	scripts/capture.sh

# Rewrite the Formula/omega.rb template from a SHA256SUMS file. The release
# workflow does this when a tag is published. brew install uses
# lcajigasm/homebrew-tap; the release opens a pull request there.
formula:
	@test -n "$(VERSION)" && test -n "$(SUMS)" || (echo "usage: make formula VERSION=0.10.0 SUMS=dist/SHA256SUMS" && exit 1)
	go run ./internal/formula/cmd -version "$(VERSION)" -sums "$(SUMS)" -file Formula/omega.rb
