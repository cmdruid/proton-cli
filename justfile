# Build the binary
build:
    bash scripts/build-hv-helpers.sh
    go build -tags=embed_hv -o proton-cli .

# Clean build artifacts
clean:
    rm -f proton-cli

# Lint and format
lint:
    gofmt -w .
    golangci-lint run ./...

# Build and run
run *args:
    go run . {{args}}

# Run unit tests (no API, no credentials - fast)
test-unit:
    go test ./cmd/... ./internal/... -count=1

# Run unit + integration tests (requires PROTON_USER and PROTON_PASSWORD)
test: test-unit
    go test ./tests/ -v -count=1 -timeout 20m

# Run a single test (or a `|`-separated regex of test names)
test-one pattern:
    go test ./tests/ -v -count=1 -run '{{pattern}}' -timeout 5m
