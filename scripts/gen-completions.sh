#!/usr/bin/env bash
# Generates shell completions into completions/, consumed by the nfpm
# packages (.deb/.rpm/.apk) and the AUR PKGBUILD during a goreleaser
# release. Run by .goreleaser.yaml's before:hooks.
#
# `go run .` (no -tags embed_hv) uses the embed stub, so no helper
# assets are needed and the completion output is identical across
# every target platform.
set -euo pipefail

cd "$(dirname "$0")/.."

mkdir -p completions
for shell in bash fish zsh; do
    go run . completion "$shell" > "completions/proton-cli.$shell"
done

echo "Generated completions:"
ls -la completions/
