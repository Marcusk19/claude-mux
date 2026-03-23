BINARY := bin/claude-mux
PREFIX ?= /usr/local
SRC := $(shell find . -name '*.go' -type f)

.PHONY: build clean install uninstall

build: $(BINARY)

$(BINARY): $(SRC) go.mod go.sum
	go build -o $(BINARY) ./cmd/claude-mux

install: $(BINARY)
	@mkdir -p $(PREFIX)/bin
	ln -sf $(abspath $(BINARY)) $(PREFIX)/bin/claude-mux
	@echo "Installed claude-mux to $(PREFIX)/bin/claude-mux"
	@echo ""
	@echo "Add to ~/.tmux.conf:"
	@echo "  run-shell '$(CURDIR)/claude-mux.tmux'"

uninstall:
	rm -f $(PREFIX)/bin/claude-mux

clean:
	rm -f $(BINARY)
