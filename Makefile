PLUGIN_ID := codex-timezone
BIN_DIR := bin

UNAME_S := $(shell uname -s)
ifeq ($(OS),Windows_NT)
PLUGIN_EXT := dll
else ifeq ($(UNAME_S),Darwin)
PLUGIN_EXT := dylib
else
PLUGIN_EXT := so
endif

PLUGIN := $(BIN_DIR)/$(PLUGIN_ID).$(PLUGIN_EXT)

.PHONY: build test vet e2e clean

build: $(PLUGIN)

$(PLUGIN): $(shell find . -name '*.go' -not -path './scripts/*') internal/ui/index.html go.mod go.sum
	mkdir -p $(BIN_DIR)
	CGO_ENABLED=1 go build -buildmode=c-shared -trimpath -o $@ .
	rm -f $(BIN_DIR)/$(PLUGIN_ID).h

test:
	go test ./...

vet:
	go vet ./...

e2e: build
	./scripts/e2e/run.sh

clean:
	rm -rf $(BIN_DIR)
