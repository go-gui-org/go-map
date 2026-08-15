.PHONY: test test-race vet lint build prepush

# Gate recipes resolve modules from go.mod, not from a go.work workspace.
# CI never sees a workspace file, so a gate that used one would answer a
# different question than "will CI go green". Development targets (if any
# are added later) may use a bare `go` to pick the workspace back up.
GO := GOWORK=off go

# golangci-lint is its own binary, so $(GO) does not cover it — but it
# honours go.work the same way the toolchain does. Without GOWORK=off it
# would type-check against sibling working copies and report breakage that
# CI, which builds the pinned versions, will never see.
LINT := GOWORK=off golangci-lint

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

# Lint. CI uses golangci-lint-action without a pinned version, so this
# target deliberately does not pin one either — whatever is installed is
# what CI would approximate. The v2 config runs the gofmt/goimports
# formatters as part of the same pass.
lint:
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
