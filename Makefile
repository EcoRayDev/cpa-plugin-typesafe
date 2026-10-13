PLUGIN_ID := typesafe
VERSION ?= 0.1.0
GOOS := $(shell go env GOOS)
GOARCH := $(shell go env GOARCH)
LIB_EXT := so
ifeq ($(GOOS),darwin)
LIB_EXT := dylib
endif
LIBRARY := dist/$(PLUGIN_ID).$(LIB_EXT)
ARCHIVE := dist/$(PLUGIN_ID)_$(VERSION)_$(GOOS)_$(GOARCH).zip

.PHONY: build test vet package
build:
	mkdir -p dist
	CGO_ENABLED=1 go build -trimpath -buildmode=c-shared -ldflags "-s -w -X main.pluginVersion=$(VERSION)" -o $(LIBRARY) ./cmd/typesafe

test:
	go test -race ./...

vet:
	go vet ./...

package: build
	go run ./scripts/package-release.go -library $(LIBRARY) -archive $(ARCHIVE) -checksum dist/checksums.txt
