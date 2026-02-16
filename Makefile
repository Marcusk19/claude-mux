BINARY := bin/claude-mux
SRC := $(shell find . -name '*.go' -type f)

.PHONY: build clean

build: $(BINARY)

$(BINARY): $(SRC) go.mod go.sum
	go build -o $(BINARY) ./cmd/claude-mux

clean:
	rm -f $(BINARY)
