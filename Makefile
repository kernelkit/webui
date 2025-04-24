VERSION    := 0.1.0
BUILD_TIME := $(shell date +%FT%T%z)
LDFLAGS    := -ldflags "-X main.Version=$(VERSION) -X main.BuildTime=$(BUILD_TIME)"
BIN        := webui

# Define installation directories with configurable prefix
PREFIX     ?= /usr
BINDIR     := $(PREFIX)/bin
SHAREDIR   := $(PREFIX)/share/$(BIN)

build:
	go build $(LDFLAGS) -o $(BIN)

clean:
	rm -f $(BIN)

distclean: clean
	rm -f templates/*~ *~

run:
	go run *.go -d -a . -s /tmp

install: build
	install -d $(DESTDIR)$(BINDIR)
	install -d $(DESTDIR)$(SHAREDIR)/assets
	install -d $(DESTDIR)$(SHAREDIR)/templates
	install -m 755 $(BIN) $(DESTDIR)$(BINDIR)/
	cp -r assets/* $(DESTDIR)$(SHAREDIR)/assets/
	cp -r templates/* $(DESTDIR)$(SHAREDIR)/templates/
	@echo "Installation complete."
	@echo "The application is now available at $(BINDIR)/$(BIN)"
	@echo "All static files have been installed in $(SHAREDIR)/"

uninstall:
	rm -f $(DESTDIR)$(BINDIR)/$(BIN)
	rm -rf $(DESTDIR)$(SHAREDIR)
	@echo "Uninstallation complete."

help:
	@echo "Infix Web GUI Makefile"
	@echo ""
	@echo "Targets:"
	@echo "  build      - Build the application"
	@echo "  run        - Run application in debug mode"
	@echo "  clean      - Remove build artifacts"
	@echo "  distclean  - Remove build artifacts and backup files"
	@echo "  install    - Install application"
	@echo "  uninstall  - Uninstall application"
	@echo "  help       - Display this help message"
	@echo ""
	@echo "Variables:"
	@echo "  PREFIX     - Installation prefix (default: $(PREFIX))"
	@echo "  DESTDIR    - Destination directory for staged installs"
	@echo "              Example: make DESTDIR=/tmp/stage install"

.PHONY: build run clean distclean install uninstall help
