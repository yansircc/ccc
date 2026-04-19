BINARY := ccc
PREFIX ?= $(HOME)/.local
BINDIR := $(PREFIX)/bin

.PHONY: build install uninstall test clean

build:
	go build -o $(BINARY) .

install: build
	mkdir -p $(BINDIR)
	cat ./$(BINARY) > $(BINDIR)/$(BINARY)
	chmod +x $(BINDIR)/$(BINARY)
	@echo "Installed: $(BINDIR)/$(BINARY)"

uninstall:
	rm -f $(BINDIR)/$(BINARY)

test:
	go test ./...

clean:
	rm -f $(BINARY)
