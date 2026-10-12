PLUGIN_ID := codex-timezone
VERSION ?= 0.1.0
REPOSITORY ?=
BIN_DIR := bin

UNAME_S := $(shell uname -s)
ifeq ($(OS),Windows_NT)
PLUGIN_EXT := dll
else ifeq ($(UNAME_S),Darwin)
PLUGIN_EXT := dylib
else
PLUGIN_EXT := so
endif

LDFLAGS := -s -w -X main.pluginVersion=$(VERSION)
ifneq ($(REPOSITORY),)
LDFLAGS += -X main.pluginRepository=$(REPOSITORY)
endif

GO_BUILD := CGO_ENABLED=1 go build -buildmode=c-shared -trimpath -ldflags "$(LDFLAGS)"
SOURCES := $(shell find . -name '*.go' -not -path './scripts/*') internal/ui/index.html go.mod go.sum

PLUGIN := $(BIN_DIR)/$(PLUGIN_ID).$(PLUGIN_EXT)
DIST := $(BIN_DIR)/$(PLUGIN_ID)-v$(VERSION).$(PLUGIN_EXT)

# Debian bookworm matches the official CPA Docker image, so the plugin links
# against the same glibc the container provides.
DOCKER_IMAGE ?= golang:1.26-bookworm
DOCKER_PLATFORM ?= linux/amd64

GOOS := $(shell go env GOOS)
GOARCH := $(shell go env GOARCH)
DIST_DIR := dist
STORE_ZIP := $(DIST_DIR)/$(PLUGIN_ID)_$(VERSION)_$(GOOS)_$(GOARCH).zip

.PHONY: build dist package registry docker-build test vet e2e e2e-store clean

build: $(PLUGIN)

$(PLUGIN): $(SOURCES)
	mkdir -p $(BIN_DIR)
	$(GO_BUILD) -o $@ .
	rm -f $(basename $@).h

# Versioned file name (codex-timezone-v<VERSION>.<ext>) lets CPA hot-swap
# upgrades without a restart.
dist: $(SOURCES)
	mkdir -p $(BIN_DIR)
	$(GO_BUILD) -o $(DIST) .
	rm -f $(basename $(DIST)).h
	@echo "built $(DIST)"

# Plugin store asset: <id>_<version>_<goos>_<goarch>.zip with the library at
# the zip root, plus a sha256sum-style checksums.txt covering every zip.
package:
	BIN_DIR=$(BIN_DIR) DIST_DIR=$(DIST_DIR) ./scripts/release/package.sh $(VERSION) $(REPOSITORY)
	python3 scripts/release/checksums.py $(DIST_DIR)

# Self-hosted store index. BASE_URL is where the zips in dist/ will be served.
registry:
	python3 scripts/store/gen_registry.py --dist $(DIST_DIR) --version $(VERSION) --base-url "$(BASE_URL)" --out $(DIST_DIR)/registry.json

docker-build:
	mkdir -p $(BIN_DIR)/$(DOCKER_PLATFORM)
	docker run --rm --platform $(DOCKER_PLATFORM) -v "$(CURDIR)":/src -w /src $(DOCKER_IMAGE) \
		sh -c 'CGO_ENABLED=1 go build -buildvcs=false -buildmode=c-shared -trimpath -ldflags "$(LDFLAGS)" -o $(BIN_DIR)/$(DOCKER_PLATFORM)/$(PLUGIN_ID)-v$(VERSION).so . && rm -f $(BIN_DIR)/$(DOCKER_PLATFORM)/*.h'
	@echo "built $(BIN_DIR)/$(DOCKER_PLATFORM)/$(PLUGIN_ID)-v$(VERSION).so"

test:
	go test ./...

vet:
	go vet ./...

e2e: build
	./scripts/e2e/run.sh

e2e-store:
	./scripts/e2e/store.sh

clean:
	rm -rf $(BIN_DIR) $(DIST_DIR)
