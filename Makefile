VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
REPO    := lcajigasm/omega
LDFLAGS := -s -w -X main.version=$(VERSION)

.PHONY: build install test lint clean formula cross

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

# Local cross-compile of the release targets (no archives).
cross:
	@mkdir -p bin
	@for pair in linux/amd64 linux/arm64 darwin/amd64 darwin/arm64 windows/amd64 windows/arm64; do \
		os=$${pair%/*}; arch=$${pair#*/}; ext=; \
		if [ "$$os" = windows ]; then ext=.exe; fi; \
		echo "$$os/$$arch"; \
		GOOS=$$os GOARCH=$$arch CGO_ENABLED=0 go build -trimpath -ldflags "$(LDFLAGS)" -o bin/omega-$$os-$$arch$$ext ./cmd/omega || exit 1; \
	done

# Rewrite Formula/omega.rb from a SHA256SUMS file. The release workflow does
# this when a tag is published; the tap repository is not touched.
formula:
	@test -n "$(VERSION)" && test -n "$(SUMS)" || (echo "usage: make formula VERSION=0.10.0 SUMS=dist/SHA256SUMS" && exit 1)
	go run ./internal/formula/cmd -version "$(VERSION)" -sums "$(SUMS)" -file Formula/omega.rb
