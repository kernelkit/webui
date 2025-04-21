VERSION    := 0.1.0
BUILD_TIME := $(shell date +%FT%T%z)
LDFLAGS    := -ldflags "-X main.Version=$(VERSION) -X main.BuildTime=$(BUILD_TIME)"
BIN        := webui

build:
	go build $(LDFLAGS) -o $(BIN)

run:
	go run *.go

clean:
	rm -f $(BIN)

distclean: clean
	rm -f templates/*~ *~

dev:
	go run *.go -d -s /tmp

install: build
	install -m 755 $(BIN) /usr/local/bin/
	mkdir -p /var/lib/misc
	@echo "Installation complete."
	@echo "The application is now available at /usr/local/bin/$(BIN)"

help:
	@echo "Infix Web GUI Makefile"
	@echo ""
	@echo "Targets:"
	@echo "  build      - Build the application"
	@echo "  run        - Run the application"
	@echo "  dev        - Run in development mode"
	@echo "  clean      - Remove build artifacts"
	@echo "  install    - Install to /usr/local/bin"
	@echo "  help       - Display this help message"

.PHONY: build run clean test dev install
