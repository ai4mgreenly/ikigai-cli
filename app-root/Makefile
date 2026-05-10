# R-J5PD-8EBD: Makefile with build/test/install/clean. clean must not
# depend on a build step. test and install may depend on build.
# R-10VP-QZBQ: produces a single statically linked binary.

BIN_DIR     := bin
BIN         := $(BIN_DIR)/ikigai-cli
PKG         := ./cmd/ikigai-cli
# R-W23P-398Z: install destination pinned to ~/.local/bin/ikigai-cli.
INSTALL_DIR := $(HOME)/.local/bin
INSTALL_BIN := $(INSTALL_DIR)/ikigai-cli

.PHONY: build test install clean

build: $(BIN)

$(BIN):
	@mkdir -p $(BIN_DIR)
	CGO_ENABLED=0 go build -ldflags='-s -w' -o $(BIN) $(PKG)

test:
	go test ./...

# R-W23P-398Z: place the built binary at ~/.local/bin/ikigai-cli,
# creating the directory if needed and replacing any existing file.
install: build
	@mkdir -p $(INSTALL_DIR)
	install -m 0755 $(BIN) $(INSTALL_BIN)

clean:
	rm -rf $(BIN_DIR)
