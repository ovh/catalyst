BUILD_DIR	:= build
VPATH			:= $(BUILD_DIR)

CC				:= go build -i -v -mod vendor
GITHASH 	:= $(shell git rev-parse --short HEAD)
GITBRANCH	:= $(shell git rev-parse --abbrev-ref HEAD)
VERSION				:= $(shell git describe --tags --candidates 1 --match '*.*')
DATE			:= $(shell TZ=UTC date -u '+%Y-%m-%dT%H:%M:%SZ UTC')
DFLAGS		:= -race
CFLAGS		:= -X 'github.com/ovh/catalyst/cmd.githash=$(GITHASH)' \
	-X 'github.com/ovh/catalyst/cmd.date=$(DATE)' \
	-X 'github.com/ovh/catalyst/cmd.gitbranch=$(GITBRANCH)' \
	-X 'github.com/ovh/catalyst/cmd.version=$(VERSION)'
CROSS			:= GOOS=linux GOARCH=amd64

FORMAT_PATHS	:= ./cmd/ ./core ./middlewares ./services ./catalyser catalyst.go
LINT_PATHS		:= ./cmd/... ./core/... ./middlewares/... ./services/... ./catalyser/... catalyst.go

rwildcard=$(foreach d,$(wildcard $1*),$(call rwildcard,$d/,$2) $(filter $(subst *,%,$2),$d))

BUILD_FILE	:= catalyst_$(VERSION)
BUILD_DEST	:= $(BUILD_DIR)/$(BUILD_FILE)

.SECONDEXPANSION:
.PHONY: all
all: format lint dist

.PHONY: init
init:
	curl -sfL https://install.goreleaser.com/github.com/golangci/golangci-lint.sh | sh -s -- -b $(GOPATH)/bin v1.17.1

.PHONY: dep
dep:
	go mod vendor -v

.PHONY: tidy
tidy:
	go mod tidy -v

build: dep catalyst.go $$(call rwildcard, ./cmd, *.go) $$(call rwildcard, ./core, *.go) $$(call rwildcard, ./catalyser, *.go)
	$(CC) $(DFLAGS) -ldflags "$(CFLAGS)" -o $(BUILD_DEST) catalyst.go

.PHONY: release
release: dep catalyst.go $$(call rwildcard, ./cmd, *.go) $$(call rwildcard, ./core, *.go) $$(call rwildcard, ./catalyser, *.go)
	$(CC) -ldflags "-s -w $(CFLAGS)" -o $(BUILD_DEST) catalyst.go

.PHONY: dist
dist: dep catalyst.go $$(call rwildcard, ./cmd, *.go) $$(call rwildcard, ./core, *.go) $$(call rwildcard, ./catalyser, *.go)
	$(CROSS) $(CC) -ldflags "-s -w $(CFLAGS)" -o $(BUILD_DEST) catalyst.go

.PHONY: lint
lint: init
	golangci-lint run --skip-dirs vendor || exit 0

.PHONY: format
format:
	gofmt -w -s $(FORMAT_PATHS)

.PHONY: dev
dev: tidy format lint build

.PHONY: clean
clean:
	rm -rf build
	rm -rf vendor
