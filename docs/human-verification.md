# Human verification

Proton's anti-abuse system sometimes asks for a CAPTCHA at login, usually on a fresh install, from a new network, or after several failed attempts.

## What happens

proton opens a small window with Proton's CAPTCHA in it. Solve it, and the command you ran retries automatically. There is nothing extra to install.

## Requirements

| Platform | Needs |
| --- | --- |
| Linux desktop | `libwebkit2gtk-4.1`, `libgtk-3`, and `glib-networking` (`apt install libwebkit2gtk-4.1-0 libgtk-3-0 glib-networking`, or your distro's equivalent). A desktop that can already run a browser has all three. |
| macOS | nothing, system WebKit is used |
| Windows | nothing, WebView2 ships with Edge |

## When the window opens empty

A window that says **"TLS support is not available"** and nothing else means the webview came up but cannot fetch anything: https needs a TLS backend, and GIO loads one as a module rather than having it built in.

Install `glib-networking` (above). If it is installed and the window still says this, the module is somewhere GIO is not looking - `GIO_EXTRA_MODULES` names extra directories to search, and a process started with a stripped environment will not have inherited it.

## When it can't run

Two situations have no window to draw in:

- **Headless machines** - servers, containers, CI, anything without a display.
- **`go install` builds** - they don't embed the helper. Use a [release binary](installation.md), or build a helper and point `PROTON_HV_HELPER` at it (below).

Either way, sign in somewhere with a display and reuse the session:

1. Run any command on a desktop machine with the same account, solving the CAPTCHA once.
2. Copy `~/.config/proton-cli/sessions/<profile>.json` to the headless machine, preserving mode `0600`.
3. Commands there use the existing session and skip login entirely.

Treat that file as a secret while copying it: it contains the session refresh token. See [Security](../SECURITY.md).

## Bringing your own helper

`PROTON_HV_HELPER` names an executable to open the CAPTCHA with, instead of the one proton carries:

```bash
PROTON_HV_HELPER=/path/to/proton-hv proton account login
```

It is read before the embedded copy, so it works in a build that has none. Two uses:

- **A build with no helper in it** - anything from `go install` - gains one. `scripts/build-hv-helpers.sh` builds it from this repo.
- **A packaged proton** can ship the helper as a real file and give it the graphics environment a webview needs, rather than putting that environment on the CLI itself, where every editor and pager proton opens would inherit it.

The helper takes a URL and prints the solved token. A path that isn't there, isn't a file, or isn't executable is reported as such rather than as verification being unavailable.
