#!/usr/bin/env bash
# Builds the proton-hv helper binary for ONE target platform and
# places it in internal/hv/assets/, ready for `//go:embed` by the
# main build (which uses `-tags embed_hv`).
#
# In CI: invoked once per (OS, arch) matrix entry on a NATIVE runner
# for that platform (Linux→ubuntu-*, macOS→macos-*, Windows→windows-*,
# windows/arm64→windows-11-arm). Cross-compiling CGO+webview reliably
# across all 6 targets isn't realistic from a single runner -
# webkit2gtk pkg-config layouts, osxcross signing, and a Windows
# header that only resolves on a case-insensitive filesystem all bring
# their own pain. Native runners avoid all of it, and a helper that
# runs on the runner that built it can be smoke tested there.
#
# Locally: defaults to the current GOOS/GOARCH and produces a single
# helper for "release-shaped" testing in `devbox shell`.
#
# KNOWN LIMITATION, Linux, devbox on a non-NixOS host: nix bakes a
# /nix/store RUNPATH into the helper, so it loads nix's webkitgtk
# rather than the host's. That webkitgtk resolves EGL drivers under
# /run/opengl-driver, which only exists on NixOS; elsewhere its render
# process dies with "Could not create default EGL display:
# EGL_BAD_PARAMETER" and the window opens but paints nothing.
#
# Unaffected: NixOS (the driver path is there), and every release
# binary (CI builds each Linux helper on a native ubuntu runner
# against apt's libwebkit2gtk-4.1-dev, with no nix involved).
#
# To render the webview on a non-NixOS host, build this helper with
# the host toolchain instead of the devbox one. Supplying nix's own
# mesa and glib-networking also works but costs ~800 MiB of closure,
# which is not worth it for an occasional check.
#
# Inputs:
#   PWD           = repo root
#   GOOS          = target OS         (default: $(go env GOOS))
#   GOARCH        = target architecture (default: $(go env GOARCH))
#   HV_TARGETS    = optional; "all" to iterate built-in target list
#   HV_OUT_DIR    = optional; default = internal/hv/assets
#
# Outputs:
#   internal/hv/assets/proton-hv-<goos>-<goarch>[.exe]
#
# Linux build deps (each Linux runner installs separately):
#   libwebkit2gtk-4.1-dev libgtk-3-dev pkg-config build-essential
#
# macOS build deps: none (Cocoa + WebKit ship with the OS).
#
# Windows build deps: llvm-mingw on PATH (https://github.com/mstorsjo/llvm-mingw).
# It is one toolchain for every Windows architecture, and the only one
# that targets aarch64: mingw-w64's GCC does not, and cgo cannot use
# MSVC. Everything else is vendored - webview_go carries the WebView2
# SDK headers in libs/mswebview2/.
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
    local out="$OUT_DIR/proton-hv-${goos}-${goarch}${suffix}"
    echo "==> building helper $goos/$goarch -> $out"

    # Reset any cross-arch CC/CGO flags from a previous iteration so
    # an arm64 build later in the loop doesn't accidentally pick them up.
    unset CC CXX CGO_CFLAGS CGO_LDFLAGS

    # pkg-config shim (Linux only): aliases webkit2gtk-4.0 → 4.1, used
    # by webview_go's hardcoded cgo line. See tools/pkgconfig/.
    if [[ "$goos" == "linux" ]]; then
        if [[ -n "${DEVBOX_PROJECT_ROOT:-}" ]]; then
            export PKG_CONFIG_PATH_FOR_TARGET="${DEVBOX_PROJECT_ROOT}/tools/pkgconfig:${PKG_CONFIG_PATH_FOR_TARGET:-}"
        fi
        export PKG_CONFIG_PATH="$PWD/tools/pkgconfig:${PKG_CONFIG_PATH:-}"
        if ! pkg-config --exists webkit2gtk-4.0; then
            echo "ERROR: pkg-config cannot find webkit2gtk-4.0 (or our 4.1 shim)" >&2
            echo "       Install libwebkit2gtk-4.1-dev (Debian/Ubuntu)" >&2
            echo "       or enter \`devbox shell\` first." >&2
            return 1
        fi
    fi

    if [[ "$goos" == "windows" ]]; then
        local triple
        case "$goarch" in
            amd64) triple=x86_64-w64-mingw32 ;;
            arm64) triple=aarch64-w64-mingw32 ;;
            *)
                echo "ERROR: no Windows toolchain for $goarch" >&2
                return 1
                ;;
        esac
        if ! command -v "$triple-clang" >/dev/null 2>&1; then
            echo "ERROR: $triple-clang not found on PATH" >&2
            echo "       Install llvm-mingw: https://github.com/mstorsjo/llvm-mingw/releases" >&2
            return 1
        fi
        export CC="$triple-clang"
        export CXX="$triple-clang++"
    fi

    # darwin/amd64 cross-compile from arm64: GitHub's macos-latest
    # runners are Apple Silicon now (Intel macos-13 runners are
    # deprecated and routinely queue for hours). macOS ships a
    # universal SDK so clang -arch x86_64 produces x86_64 Mach-O
    # binaries while running natively on arm64. CGO picks up the
    # arch via CC / CGO_*FLAGS.
    if [[ "$goos" == "darwin" && "$goarch" == "amd64" ]]; then
        host_arch="$(uname -m)"
        if [[ "$host_arch" == "arm64" ]]; then
            export CC="cc -arch x86_64"
            export CXX="c++ -arch x86_64"
            export CGO_CFLAGS="-arch x86_64"
            export CGO_LDFLAGS="-arch x86_64"
        fi
    fi

    GOOS="$goos" GOARCH="$goarch" CGO_ENABLED=1 \
        go build \
            -tags webview \
            -trimpath \
            -ldflags '-s -w' \
            -o "$out" \
            ./cmd/proton-hv/
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
        build_one windows arm64
        ;;
    *)
        build_one "${GOOS:-$GOOS_DEFAULT}" "${GOARCH:-$GOARCH_DEFAULT}"
        ;;
esac

echo ""
echo "Helper assets in $OUT_DIR:"
ls -la "$OUT_DIR"/proton-hv-*
