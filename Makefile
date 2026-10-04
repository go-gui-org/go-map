.PHONY: test test-race vet lint lint-bin build prepush

# Gate recipes resolve modules from go.mod, not from a go.work workspace.
# CI never sees a workspace file, so a gate that used one would answer a
# different question than "will CI go green". Development targets (if any
# are added later) may use a bare `go` to pick the workspace back up.
GO := GOWORK=off go

# Repo-local bin for the pinned linter. The pinned VERSION itself lives in
# tools/lint/go.mod -- see the $(LINT_BIN) rule below. `make lint` and CI
# both build from that file, so a local pass and a CI pass run one version.
LINT_DIR = $(CURDIR)/.bin
LINT_BIN = $(LINT_DIR)/golangci-lint

# golangci-lint is its own binary, so $(GO) does not cover it — but it
# honours go.work the same way the toolchain does. Without GOWORK=off it
# would type-check against sibling working copies and report breakage that
# CI, which builds the pinned versions, will never see.
LINT := GOWORK=off $(LINT_BIN)

# Run the test suite. Mirrors the CI test job's non-race half (macOS runner).
test:
	$(GO) test ./...

# Race-enabled tests. CI runs -race on its Linux runner only; running it
# here covers that leg from any host.
test-race:
	$(GO) test -race -count=1 ./...

# Static analysis. Mirrors the CI vet job.
vet:
	$(GO) vet ./...

# Build the pinned golangci-lint into .bin/. It rebuilds only when
# tools/lint/go.mod or go.sum change. GOWORK=off keeps a local go.work out
# of the build. GOOS/GOARCH/CGO_ENABLED are cleared so a caller that sets
# them to pick a lint target does not cross-compile the linter itself into
# a binary this host cannot run.
$(LINT_BIN): tools/lint/go.mod tools/lint/go.sum
	GOWORK=off GOOS= GOARCH= CGO_ENABLED=0 GOFLAGS= GOBIN=$(LINT_DIR) \
	  go -C tools/lint install \
	  github.com/golangci/golangci-lint/v2/cmd/golangci-lint

lint-bin: $(LINT_BIN)

# Lint at the version pinned in tools/lint. CI runs this same target. The
# v2 config runs the gofmt/goimports formatters as part of the same pass.
lint: $(LINT_BIN)
	$(LINT) run ./...

build:
	$(GO) build ./...

# Recommended full local validation before pushing (issue #314).
# Approximates the CI matrix from one host: race tests, vet, lint, build.
# Aborts on the first failing target.
#
# Omissions vs CI, by design: the OS matrix itself — CI runs the suite on
# both ubuntu-latest and macos-latest, and only the host's own platform is
# exercised here.
prepush: test-race vet lint build
