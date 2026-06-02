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

# Format Go sources in place.
fmt:
	gofmt -s -w .

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

.PHONY: default build test testacc lint fmt generate docs tidy
