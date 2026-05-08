#!/usr/bin/env bash
# Builds the proton-cli-hv helper binary for ONE target platform and
# places it in internal/hv/assets/, ready for `//go:embed` by the
# main build (which uses `-tags embed_hv`).
#
# In CI: invoked once per (OS, arch) matrix entry on a NATIVE runner
# for that platform (Linux→ubuntu-*, macOS→macos-*, Windows→windows-*).
# Cross-compiling CGO+webview reliably across all 5 targets isn't
# realistic from a single runner — webkit2gtk pkg-config layouts,
# osxcross signing, and Windows mingw all bring their own pain. Native
# runners avoid all of it.
#
# Locally: defaults to the current GOOS/GOARCH and produces a single
# helper for "release-shaped" testing in `devbox shell`.
#
# Inputs:
#   PWD           = repo root
#   GOOS          = target OS         (default: $(go env GOOS))
#   GOARCH        = target architecture (default: $(go env GOARCH))
#   HV_TARGETS    = optional; "all" to iterate built-in target list
#   HV_OUT_DIR    = optional; default = internal/hv/assets
#
# Outputs:
#   internal/hv/assets/proton-cli-hv-<goos>-<goarch>[.exe]
#
# Linux build deps (each Linux runner installs separately):
#   libwebkit2gtk-4.1-dev libgtk-3-dev pkg-config build-essential
#
# macOS build deps: none (Cocoa + WebKit ship with the OS).
#
# Windows build deps: none beyond mingw-gcc (webview_go vendors the
# WebView2 SDK headers in libs/mswebview2/).
set -euo pipefail

cd "$(dirname "$0")/.."

OUT_DIR="${HV_OUT_DIR:-internal/hv/assets}"
mkdir -p "$OUT_DIR"

# Default to current platform.
GOOS_DEFAULT="$(go env GOOS)"
GOARCH_DEFAULT="$(go env GOARCH)"

build_one() {
    local goos="$1" goarch="$2"
    local suffix=""
    if [[ "$goos" == "windows" ]]; then
        suffix=".exe"
    fi
    local out="$OUT_DIR/proton-cli-hv-${goos}-${goarch}${suffix}"
    echo "==> building helper $goos/$goarch -> $out"

    # pkg-config shim (Linux only): aliases webkit2gtk-4.0 → 4.1, used
    # by webview_go's hardcoded cgo line. See tools/pkgconfig/.
    if [[ "$goos" == "linux" ]]; then
        if [[ -n "${DEVBOX_PROJECT_ROOT:-}" ]]; then
            export PKG_CONFIG_PATH_FOR_TARGET="${DEVBOX_PROJECT_ROOT}/tools/pkgconfig:${PKG_CONFIG_PATH_FOR_TARGET:-}"
        fi
        export PKG_CONFIG_PATH="$(pwd)/tools/pkgconfig:${PKG_CONFIG_PATH:-}"
        if ! pkg-config --exists webkit2gtk-4.0; then
            echo "ERROR: pkg-config cannot find webkit2gtk-4.0 (or our 4.1 shim)" >&2
            echo "       Install libwebkit2gtk-4.1-dev (Debian/Ubuntu)" >&2
            echo "       or enter \`devbox shell\` first." >&2
            return 1
        fi
    fi

    GOOS="$goos" GOARCH="$goarch" CGO_ENABLED=1 \
        go build \
            -tags webview \
            -trimpath \
            -ldflags '-s -w' \
            -o "$out" \
            ./cmd/proton-cli-hv/
    ls -la "$out"
}

case "${HV_TARGETS:-}" in
    all)
        # Local "build everything" mode (only useful inside devbox on
        # a Linux dev machine; cross-CGO will likely fail for non-Linux
        # targets without extra cross-toolchains).
        build_one linux  amd64
        build_one linux  arm64
        build_one darwin amd64
        build_one darwin arm64
        build_one windows amd64
        ;;
    *)
        build_one "${GOOS:-$GOOS_DEFAULT}" "${GOARCH:-$GOARCH_DEFAULT}"
        ;;
esac

echo ""
echo "Helper assets in $OUT_DIR:"
ls -la "$OUT_DIR/" | grep -v '^d'
