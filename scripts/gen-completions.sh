#!/usr/bin/env bash
# Generates shell completions into completions/, consumed by the nfpm
# packages (.deb/.rpm/.apk), the AUR PKGBUILD and the Homebrew cask
# during a goreleaser release. Run by .goreleaser.yaml's before:hooks.
#
# `go run ./cmd/proton` (no -tags embed_hv) uses the embed stub, so no
# helper assets are needed and the completion output is identical
# across every target platform.
#
# One script per shell serves both names: the generator registers
# `proton-cli` alongside `proton`. Fish is the exception - it autoloads
# a file named after the command being typed, so the alias needs a file
# of its own that borrows the real one.
set -euo pipefail

cd "$(dirname "$0")/.."

# Cleared first: the whole directory is copied into every archive and package,
# so a file left behind by an earlier run would ship.
rm --recursive --force completions
mkdir -p completions
for shell in bash fish zsh; do
    go run ./cmd/proton completion "$shell" > "completions/proton.$shell"
done
printf 'complete -c proton-cli -w proton\n' > completions/proton-cli.fish

echo "Generated completions:"
ls -la completions/
