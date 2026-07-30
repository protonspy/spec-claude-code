# scc — build, test, and npm release tasks.
#
# Day-to-day:
#   make build                       # local binary -> ./scc
#   make check                       # gofmt + vet + race tests (the CI gate)
#
# Release (manual — dispatches the release workflow; CI builds + publishes npm):
#   make release VERSION=v0.1.0      # = gh workflow run release.yml --field version=v0.1.0
#
# Manual npm publish (bootstrap / fallback, when CI can't do it):
#   make dist      VERSION=v0.1.0    # cross-compile all 6 targets into dist/
#   make npm-build VERSION=v0.1.0    # assemble npm/dist/ (7 packages) from those
#   make npm-dry-run                 # validate every package without publishing
#   make npm-publish [OTP=123456]    # publish (skips already-published)
#
# Auth for a manual publish: either an Automation token
#   npm config set //registry.npmjs.org/:_authToken <token>
# or pass a fresh 2FA code:  make npm-publish OTP=123456

SHELL     := bash
MODULE    := github.com/protonspy/spec-claude-code
BIN       := scc
VERSION   ?= dev
LDFLAGS   := -s -w -X $(MODULE)/internal/cli.version=$(VERSION)
DIST      ?= dist
ARTIFACTS ?= $(DIST)

# GOOS/GOARCH targets shipped to npm — must match npm/scripts/build-packages.mjs
# and the release.yml build matrix.
PLATFORMS := linux/amd64 linux/arm64 darwin/amd64 darwin/arm64 windows/amd64 windows/arm64

.DEFAULT_GOAL := help

.PHONY: help
help: ## Show this help
	@grep -hE '^[a-zA-Z0-9_-]+:.*## ' $(MAKEFILE_LIST) \
	  | awk 'BEGIN{FS=":.*## "}{printf "  \033[36m%-14s\033[0m %s\n",$$1,$$2}'

.PHONY: build
build: ## Build the binary locally (-> ./scc); set VERSION to stamp it
	go build -trimpath -ldflags '$(LDFLAGS)' -o $(BIN) ./cmd/$(BIN)

.PHONY: test
test: ## Run tests (race + coverage)
	go test -race -coverprofile=coverage.out ./...

.PHONY: fmt
fmt: ## Fail if any file is not gofmt-clean
	@unformatted=$$(gofmt -l .); \
	if [ -n "$$unformatted" ]; then echo "not gofmt-clean:"; echo "$$unformatted"; exit 1; fi; \
	echo "gofmt clean"

.PHONY: vet
vet: ## go vet ./...
	go vet ./...

.PHONY: check
check: fmt vet test ## Run the full CI gate (gofmt + vet + race tests)

# require-version fails fast when the release VERSION is left at the 'dev'
# default. Without it, dist/npm-build produce 'dev'-named tarballs (and an
# invalid npm semver), which only surfaces later as a cryptic `tar: Cannot open`
# when npm-build can't find a matching artifact.
.PHONY: require-version
require-version:
	@case '$(VERSION)' in v*.*.*) : ;; *) echo "set a release VERSION like v0.1.1 (got '$(VERSION)'); e.g. make $(MAKECMDGOALS) VERSION=v0.1.1" >&2; exit 1 ;; esac

.PHONY: dist
dist: require-version ## Cross-compile every npm target into $(DIST)/ (set VERSION=vX.Y.Z)
	@rm -rf '$(DIST)' && mkdir -p '$(DIST)'
	@set -euo pipefail; for p in $(PLATFORMS); do \
	  goos=$${p%/*}; goarch=$${p#*/}; bin=$(BIN); [ "$$goos" = windows ] && bin=$(BIN).exe; \
	  echo ">> $$goos/$$goarch"; \
	  GOOS=$$goos GOARCH=$$goarch CGO_ENABLED=0 go build -trimpath -ldflags '$(LDFLAGS)' -o '$(DIST)'/$$bin ./cmd/$(BIN); \
	  name=$(BIN)_$(VERSION)_$${goos}_$${goarch}; \
	  if [ "$$goos" = windows ]; then (cd '$(DIST)' && zip -q $$name.zip $$bin); \
	  else (cd '$(DIST)' && tar -czf $$name.tar.gz $$bin); fi; \
	  rm -f '$(DIST)'/$$bin; \
	done; \
	echo "artifacts in $(DIST)/"

.PHONY: npm-build
npm-build: require-version ## Assemble npm/dist/ from the artifacts (VERSION=vX.Y.Z; ARTIFACTS=dir; needs `make dist`)
	node npm/scripts/build-packages.mjs '$(VERSION)' '$(ARTIFACTS)'

.PHONY: npm-dry-run
npm-dry-run: ## Dry-run publish every assembled package
	@set -euo pipefail; \
	if [ ! -f npm/dist/$(BIN)/package.json ]; then echo "npm/dist not assembled — run: make npm-build VERSION=vX.Y.Z first" >&2; exit 1; fi; \
	for d in npm/dist/$(BIN)-*/ npm/dist/$(BIN)/; do \
	  [ -f "$$d/package.json" ] || continue; \
	  echo "== $$d"; npm publish "$$d" --access public --dry-run; done

.PHONY: npm-publish
npm-publish: ## Publish the assembled packages, skips already-published; OTP=123456 if 2FA
	@set -euo pipefail; \
	if [ ! -f npm/dist/$(BIN)/package.json ]; then echo "npm/dist not assembled — run: make dist VERSION=vX.Y.Z && make npm-build VERSION=vX.Y.Z" >&2; exit 1; fi; \
	otp=; if [ -n "$(OTP)" ]; then otp="--otp=$(OTP)"; fi; \
	for d in npm/dist/$(BIN)-*/ npm/dist/$(BIN)/; do \
	  [ -f "$$d/package.json" ] || continue; \
	  name=$$(cd "$$d" && node -p "require('./package.json').name"); \
	  ver=$$(cd "$$d" && node -p "require('./package.json').version"); \
	  if npm view "$$name@$$ver" version >/dev/null 2>&1; then \
	    echo "skip $$name@$$ver (already published)"; continue; fi; \
	  echo ">> publishing $$name@$$ver"; npm publish "$$d" --access public $$otp; \
	done

.PHONY: release
release: require-version ## Dispatch the manual release workflow (CI builds binaries + publishes npm) for VERSION
	gh workflow run release.yml --field version='$(VERSION)'
	@echo "dispatched release $(VERSION) — track it with: gh run list --workflow=release.yml"

.PHONY: clean
clean: ## Remove build artifacts (dist/, npm/dist/, ./scc, coverage)
	rm -rf '$(DIST)' npm/dist $(BIN) $(BIN).exe coverage.out
