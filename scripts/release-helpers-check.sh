#!/usr/bin/env bash
# Pre-build sanity for goreleaser: verifies that every helper binary
# that //go:embed expects is actually present in internal/hv/assets/.
#
# Run by .goreleaser.yaml's before:hooks. The CI release workflow
# populates internal/hv/assets/ by downloading the per-platform
# artifacts the matrix-build job produced; if anything went missing
# we want to fail loud here rather than ship a binary with garbage
# embedded bytes.
set -euo pipefail

cd "$(dirname "$0")/.."

ASSETS=internal/hv/assets
expected=(
    proton-cli-hv-linux-amd64
    proton-cli-hv-linux-arm64
    proton-cli-hv-darwin-amd64
    proton-cli-hv-darwin-arm64
    proton-cli-hv-windows-amd64.exe
)

missing=()
for f in "${expected[@]}"; do
    if [[ ! -s "$ASSETS/$f" ]]; then
        missing+=("$f")
    fi
done

if [[ ${#missing[@]} -gt 0 ]]; then
    echo "ERROR: helper binaries missing from $ASSETS/:" >&2
    for f in "${missing[@]}"; do echo "  - $f" >&2; done
    echo "" >&2
    echo "These should be produced by the build-helpers matrix job in" >&2
    echo ".github/workflows/release.yml and downloaded as artifacts" >&2
    echo "before this goreleaser run." >&2
    exit 1
fi

echo "All ${#expected[@]} helper binaries present:"
ls -la "$ASSETS/" | grep -v '^d'
