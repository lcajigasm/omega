VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
REPO    := lcajigasm/omega
LDFLAGS := -s -w -X main.version=$(VERSION)

.PHONY: build install test lint clean formula

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

# Point Formula/omega.rb at the GitHub tarball of tag v$(VERSION).
# Push the tag first: git tag v0.1.0 && git push origin v0.1.0
formula:
	@test -n "$(VERSION)" || (echo "usage: make formula VERSION=0.1.0" && exit 1)
	@url="https://github.com/$(REPO)/archive/refs/tags/v$(VERSION).tar.gz"; \
	sha=$$(curl -fsSL "$$url" | shasum -a 256 | cut -d' ' -f1); \
	sed -i '' -e "s|^  url \".*\"|  url \"$$url\"|" -e "s|^  sha256 \".*\"|  sha256 \"$$sha\"|" Formula/omega.rb; \
	echo "Formula/omega.rb -> v$(VERSION) ($$sha)"
