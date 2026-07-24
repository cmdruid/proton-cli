#!/bin/sh
# proton-cli installer.
#
# Downloads the latest proton-cli release for your OS/architecture from GitHub
# Releases, verifies its SHA-256 checksum, and installs the binary. No Go
# toolchain and no package manager required.
#
# Usage:
#   curl -fsSL https://raw.githubusercontent.com/roman-16/proton-cli/main/scripts/install.sh | sh
#
# Options (after `... | sh -s --`):
#   --version <X.Y.Z>   Install a specific release (default: latest).
#   --install-dir <dir> Install into <dir> (default: ~/.local/bin).
#   --help              Show this help.
#
# Environment overrides:
#   PROTON_CLI_VERSION      Same as --version.
#   PROTON_CLI_INSTALL_DIR  Same as --install-dir.
#
# Windows: use the PowerShell installer instead:
#   irm https://raw.githubusercontent.com/roman-16/proton-cli/main/scripts/install.ps1 | iex
set -eu

REPO="roman-16/proton-cli"
BIN="proton-cli"
VERSION="${PROTON_CLI_VERSION:-}"
INSTALL_DIR="${PROTON_CLI_INSTALL_DIR:-$HOME/.local/bin}"

main() {
	parse_args "$@"
	need_cmd uname
	need_downloader

	os="$(detect_os)"
	arch="$(detect_arch)"
	asset="${BIN}_${os}_${arch}"

	if [ -n "$VERSION" ]; then
		base="https://github.com/$REPO/releases/download/v${VERSION#v}"
	else
		base="https://github.com/$REPO/releases/latest/download"
	fi

	tmp="$(mktemp -d)"
	trap 'rm -rf "$tmp"' EXIT INT HUP TERM

	info "Downloading ${asset}${VERSION:+ (v${VERSION#v})}..."
	download "$base/$asset" "$tmp/$asset" ||
		die "download failed: $base/$asset (no prebuilt binary for $os/$arch in this release?)"
	download "$base/checksums.txt" "$tmp/checksums.txt" ||
		die "could not download checksums.txt from $base"

	verify_checksum "$tmp/$asset" "$tmp/checksums.txt" "$asset"
	info "Checksum verified."

	mkdir -p "$INSTALL_DIR"
	install -m 0755 "$tmp/$asset" "$INSTALL_DIR/$BIN" 2>/dev/null ||
		die "could not install to $INSTALL_DIR (set --install-dir to a writable directory)"

	installed="$("$INSTALL_DIR/$BIN" --version 2>/dev/null || echo "$BIN")"
	success "Installed $installed to $INSTALL_DIR/$BIN"

	check_path
	completions_hint
	info "Uninstall any time with: proton-cli uninstall"
}

parse_args() {
	while [ $# -gt 0 ]; do
		case "$1" in
		--version)
			[ $# -ge 2 ] || die "--version requires an argument"
			VERSION="$2"
			shift 2
			;;
		--version=*) VERSION="${1#*=}"; shift ;;
		--install-dir)
			[ $# -ge 2 ] || die "--install-dir requires an argument"
			INSTALL_DIR="$2"
			shift 2
			;;
		--install-dir=*) INSTALL_DIR="${1#*=}"; shift ;;
		-h | --help) usage; exit 0 ;;
		*) die "unknown option: $1 (see --help)" ;;
		esac
	done
}

usage() {
	sed -n '2,26p' "$0" 2>/dev/null | sed 's/^# \{0,1\}//' || true
}

detect_os() {
	case "$(uname -s)" in
	Linux) echo linux ;;
	Darwin) echo darwin ;;
	MINGW* | MSYS* | CYGWIN* | Windows_NT)
		die "Windows is not supported by this script. Use the PowerShell installer:
  irm https://raw.githubusercontent.com/$REPO/main/scripts/install.ps1 | iex
or install via winget: winget install Roman-16.ProtonCLI" ;;
	*) die "unsupported OS: $(uname -s)" ;;
	esac
}

detect_arch() {
	case "$(uname -m)" in
	x86_64 | amd64) echo amd64 ;;
	aarch64 | arm64) echo arm64 ;;
	*) die "unsupported architecture: $(uname -m)" ;;
	esac
}

verify_checksum() {
	file="$1"; sums="$2"; name="$3"
	expected="$(awk -v f="$name" '$2 == f { print $1; exit }' "$sums")"
	[ -n "$expected" ] || die "no checksum entry for $name in checksums.txt"

	if command -v sha256sum >/dev/null 2>&1; then
		actual="$(sha256sum "$file" | awk '{print $1}')"
	elif command -v shasum >/dev/null 2>&1; then
		actual="$(shasum -a 256 "$file" | awk '{print $1}')"
	elif command -v openssl >/dev/null 2>&1; then
		actual="$(openssl dgst -sha256 "$file" | awk '{print $NF}')"
	else
		die "no SHA-256 tool found (need sha256sum, shasum, or openssl)"
	fi

	[ "$expected" = "$actual" ] ||
		die "checksum mismatch for $name (expected $expected, got $actual)"
}

check_path() {
	case ":$PATH:" in
	*":$INSTALL_DIR:"*) ;;
	*)
		warn "$INSTALL_DIR is not on your PATH. Add it to your shell profile:"
		# shellcheck disable=SC2016 # $PATH is meant to stay literal for pasting.
		printf '\n    export PATH="%s:$PATH"\n\n' "$INSTALL_DIR" >&2
		;;
	esac
}

completions_hint() {
	info "Enable shell completions with: proton-cli completion bash|zsh|fish"
}

need_downloader() {
	if command -v curl >/dev/null 2>&1; then
		DL=curl
	elif command -v wget >/dev/null 2>&1; then
		DL=wget
	else
		die "need curl or wget"
	fi
}

download() {
	# download URL OUTFILE
	if [ "$DL" = curl ]; then
		curl -fsSL --proto '=https' --tlsv1.2 --retry 3 --connect-timeout 10 -o "$2" "$1"
	else
		wget -q --https-only -O "$2" "$1"
	fi
}

need_cmd() { command -v "$1" >/dev/null 2>&1 || die "need '$1'"; }

if [ -t 2 ]; then
	C_GREEN="$(printf '\033[32m')"; C_YELLOW="$(printf '\033[33m')"; C_RED="$(printf '\033[31m')"; C_OFF="$(printf '\033[0m')"
else
	C_GREEN=""; C_YELLOW=""; C_RED=""; C_OFF=""
fi

info() { printf '  %s\n' "$1" >&2; }
success() { printf '  %s✓%s %s\n' "$C_GREEN" "$C_OFF" "$1" >&2; }
warn() { printf '  %s!%s %s\n' "$C_YELLOW" "$C_OFF" "$1" >&2; }
die() { printf '  %s✗%s %s\n' "$C_RED" "$C_OFF" "$1" >&2; exit 1; }

main "$@"
