# Human verification

Proton's anti-abuse system sometimes asks for a CAPTCHA at login, usually on a fresh install, from a new network, or after several failed attempts.

## What happens

proton-cli opens a small window with Proton's CAPTCHA in it. Solve it, and the command you ran retries automatically. There is nothing extra to install.

## Requirements

| Platform | Needs |
| --- | --- |
| Linux desktop | `libwebkit2gtk-4.1` and `libgtk-3` (`apt install libwebkit2gtk-4.1-0 libgtk-3-0`, or your distro's equivalent) |
| macOS | nothing, system WebKit is used |
| Windows | nothing, WebView2 ships with Edge |

## When it can't run

Two situations have no window to draw in:

- **Headless machines** - servers, containers, CI, anything without a display.
- **`go install` builds** - they don't embed the helper. Use a [release binary](installation.md) instead.

Either way, sign in somewhere with a display and reuse the session:

1. Run any command on a desktop machine with the same account, solving the CAPTCHA once.
2. Copy `~/.config/proton-cli/sessions/<profile>.json` to the headless machine, preserving mode `0600`.
3. Commands there use the existing session and skip login entirely.

Treat that file as a secret while copying it: it contains the session refresh token. See [Security](../SECURITY.md).
