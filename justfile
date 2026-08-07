# Build the binary
build:
    bash scripts/build-hv-helpers.sh
    go build -tags=embed_hv -o proton-cli .

# Clean build artifacts
clean:
    rm -f proton-cli

# Regenerate the README demo images by recording a real session
demo: build
    #!/usr/bin/env bash
    set -euo pipefail
    render() { freeze /tmp/proton-cli-demo.ansi --config "scripts/terminal-demo/$1.json" --output "assets/demo-$1.svg"; }
    # freeze takes the color of text that carries no ANSI color from a syntax
    # theme, and a theme that defines none leaves an invalid fill behind, so
    # each panel states its own default instead.
    default_text() { sed --in-place "s|\\(<g font-family=[^>]*\\)fill=\"[^\"]*\"|\\1fill=\"$2\"|" "assets/demo-$1.svg"; }
    # A pty redraws the progress bar with carriage returns; keep the last frame
    # of each line so the recording reads like the finished screen.
    script --quiet --command "bash scripts/terminal-demo/record.sh" --return /dev/null \
        | sed --expression 's/\r$//' --expression 's/.*\r//' > /tmp/proton-cli-demo.ansi
    render dark
    default_text dark "#FFFFFF"
    render light
    default_text light "#0C0C14"

# Prepare the demo account with the data the recording shows
demo-seed: build
    bash scripts/terminal-demo/seed.sh

# Lint and format
lint:
    gofmt -w .
    golangci-lint run ./...

# Build and run
run *args:
    go run . {{args}}

# Unit, golden and conformance tests: no API, no credentials, seconds not minutes
test-fast:
    go test ./cmd/... ./internal/... -count=1

# Regenerate the golden files that pin every response's exact bytes
golden:
    go test ./internal/ui/ -update -count=1

# Regenerate the command reference from the command tree
docs:
    go run ./scripts/gendocs

# Run everything, including the live-API integration suite
# (requires PROTON_USER and PROTON_PASSWORD)
test: test-fast
    go test ./tests/ -v -count=1 -timeout 30m

# Run a single test (or a `|`-separated regex of test names)
test-one pattern:
    go test ./tests/ -v -count=1 -run '{{pattern}}' -timeout 5m
