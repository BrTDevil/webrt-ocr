BIN     := webrt-ocr
PREFIX  ?= /usr/local
BINDIR  := $(PREFIX)/bin
LDFLAGS := -s -w

.PHONY: build install uninstall test release clean

SRC     := $(wildcard *.go) go.mod go.sum

build: $(BIN)

# recompilează doar când s-au schimbat sursele
$(BIN): $(SRC)
	go build -trimpath -ldflags "$(LDFLAGS)" -o $(BIN) .

# compilează ca utilizatorul curent; sudo e cerut doar dacă BINDIR nu e inscriptibil
SUDO     = $(shell [ -w $(BINDIR) ] || [ ! -e $(BINDIR) -a -w $(PREFIX) ] || echo sudo)

install: $(BIN)
	$(SUDO) install -d $(BINDIR)
	$(SUDO) install -m 0755 $(BIN) $(BINDIR)/$(BIN)
	@echo "Instalat: $(BINDIR)/$(BIN)"

uninstall:
	$(SUDO) rm -f $(BINDIR)/$(BIN)

test:
	go vet ./...
	go test ./...

release:
	GOOS=linux   GOARCH=amd64 go build -trimpath -ldflags "$(LDFLAGS)" -o dist/$(BIN)-linux-amd64 .
	GOOS=linux   GOARCH=arm64 go build -trimpath -ldflags "$(LDFLAGS)" -o dist/$(BIN)-linux-arm64 .
	GOOS=darwin  GOARCH=arm64 go build -trimpath -ldflags "$(LDFLAGS)" -o dist/$(BIN)-macos-arm64 .
	GOOS=darwin  GOARCH=amd64 go build -trimpath -ldflags "$(LDFLAGS)" -o dist/$(BIN)-macos-amd64 .
	GOOS=windows GOARCH=amd64 go build -trimpath -ldflags "$(LDFLAGS)" -o dist/$(BIN)-windows-amd64.exe .

clean:
	rm -rf $(BIN) dist/
