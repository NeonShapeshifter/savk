GO ?= go
VERSION ?= 0.1.5
COMMIT ?= unknown
BUILD_DATE ?= $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")
GOOS ?= $(shell $(GO) env GOOS)
GOARCH ?= $(shell $(GO) env GOARCH)
BIN_DIR ?= bin
DIST_DIR ?= dist

LDFLAGS = -X main.version=$(VERSION) -X main.commit=$(COMMIT) -X main.buildDate=$(BUILD_DATE)

.PHONY: build test integration clean dist

build:
	mkdir -p $(BIN_DIR)
	CGO_ENABLED=0 $(GO) build -trimpath -ldflags "$(LDFLAGS)" -o $(BIN_DIR)/savk ./cmd/savk

test:
	$(GO) test ./...

integration:
	test "$$SAVK_RUN_SYSTEMD_INTEGRATION" = "1"
	$(GO) test ./cmd/savk -run TestSystemdIntegrationSmoke -count=1

dist:
	mkdir -p $(DIST_DIR)
	CGO_ENABLED=0 GOOS=$(GOOS) GOARCH=$(GOARCH) $(GO) build -trimpath -ldflags "-s -w $(LDFLAGS)" -o $(DIST_DIR)/savk-$(VERSION)-$(GOOS)-$(GOARCH) ./cmd/savk
	tar -C $(DIST_DIR) -czf $(DIST_DIR)/savk-$(VERSION)-$(GOOS)-$(GOARCH).tar.gz savk-$(VERSION)-$(GOOS)-$(GOARCH)
	cd $(DIST_DIR) && sha256sum savk-$(VERSION)-$(GOOS)-$(GOARCH) savk-$(VERSION)-$(GOOS)-$(GOARCH).tar.gz > SHA256SUMS

clean:
	rm -rf $(BIN_DIR) $(DIST_DIR)
