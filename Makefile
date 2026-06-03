NAME    := terraform-provider-unifi

default: build

# Compile and install the provider into $GOBIN.
build:
	go install ./...

# Unit tests — no controller required (the bulk of coverage lives here).
test:
	go test ./... -timeout 120s

# Acceptance tests — needs a controller (Docker for legacy/v2; HIL for Integration).
# Never run against production. See docs/testing.md.
testacc:
	TF_ACC=1 go test ./... -timeout 60m

# Static analysis. golangci-lint must be installed (CI installs it).
lint:
	go vet ./...
	golangci-lint run ./...

# Format Go sources and example HCL in place.
fmt:
	gofmt -s -w .
	terraform fmt -recursive examples/

# Check example HCL formatting (mirrors the CI `terraform fmt` gate).
tffmt:
	terraform fmt -check -recursive examples/

# Scan for known vulnerabilities reachable from our call graph — deps and the Go
# standard library (mirrors the CI govulncheck gate). GOTOOLCHAIN pins the scan
# to the security-patched 1.26.4 stdlib (see test.yml); pinned tool version;
# advisory DB fetched from vuln.go.dev.
vulncheck:
	GOTOOLCHAIN=go1.26.4 go run golang.org/x/vuln/cmd/govulncheck@v1.3.0 ./...

# Regenerate everything (docs + wire codegen). Needs the sibling ../unifi-specs
# repo for the wire `go:generate` directives — local dev only, never CI.
generate:
	go generate ./...

# Regenerate ONLY the provider docs (tfplugindocs). No wire codegen, no
# ../unifi-specs needed — mirrors the CI docs-freshness gate in test.yml.
docs:
	go tool tfplugindocs generate --provider-name unifi

tidy:
	go mod tidy

# Run the full PR gate locally before pushing — mirrors .github/workflows/test.yml
# (build/vet, gofmt, go mod tidy, govulncheck, terraform fmt, docs, lint, unit).
check: build
	go vet ./...
	test -z "$$(gofmt -s -l .)" || { echo "gofmt: run 'make fmt'"; exit 1; }
	go mod tidy && git diff --exit-code go.mod go.sum
	$(MAKE) vulncheck
	$(MAKE) tffmt
	$(MAKE) docs && git diff --exit-code docs/
	golangci-lint run ./...
	$(MAKE) test

.PHONY: default build test testacc lint fmt tffmt vulncheck generate docs tidy check
