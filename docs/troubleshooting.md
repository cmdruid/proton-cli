# Troubleshooting

What to do when a command fails: CAPTCHAs at login, signing in on a machine with no display, a session that expired, Proton asking for your password again, a change that doesn't show up yet, and rate limits.

## What the exit code is telling you

Errors say what happened and what to try, so read the message first. The code is what a script reads.

| Code | Means | Do |
| --- | --- | --- |
| `1` | Something you passed was wrong | Fix the command; nothing was sent |
| `2` | Authentication failed | Sign in again, or fix the password |
| `3` | Not found | Check the reference, or run the matching `list` |
| `4` | Ambiguous, or a conflict | Narrow the term, or use the ID it printed |
| `5` | Network or server problem | Wait and retry; this is not your command's fault |
| `130` | Cancelled with `Ctrl+C` | Nothing to do |

The difference between `2` and `5` matters most to a scheduled job: `2` means fix the credential, `5` means come back later. A rate limit is `5`.

## Proton asks for a CAPTCHA

Proton's anti-abuse system sometimes asks for human verification at login - usually on a fresh install, from a new network, or after several failed attempts.

proton opens a small window with Proton's CAPTCHA in it. Solve it, and the command you ran retries automatically. There is nothing extra to install.

| Platform | Needs |
| --- | --- |
| Linux desktop | `libwebkit2gtk-4.1`, `libgtk-3` and `glib-networking` (`apt install libwebkit2gtk-4.1-0 libgtk-3-0 glib-networking`, or your distro's equivalent). A desktop that can already run a browser has all three. |
| macOS | nothing, system WebKit is used |
| Windows | nothing, WebView2 ships with Edge |

### The window opens empty

A window that says **"TLS support is not available"** and nothing else means the webview came up but cannot fetch anything: https needs a TLS backend, and GIO loads one as a module rather than having it built in.

Install `glib-networking` (above). If it is installed and the window still says this, the module is somewhere GIO is not looking - `GIO_EXTRA_MODULES` names extra directories to search, and a process started with a stripped environment will not have inherited it.

### There is no window to draw in

Two situations have none:

- **Headless machines** - servers, containers, CI, anything without a display. See below.
- **`go install` builds** - they don't embed the helper. Use a [release binary](installation.md), or [bring your own helper](#bringing-your-own-captcha-helper).

## A security key that nothing finds

`login` reports a key it cannot see and a key it cannot open as two different things, because they have different fixes.

**"No security key is connected to this machine."** Nothing answered on USB. Plug the key in - the one you registered with Proton - and run the command again. A passkey held in a phone cannot answer here at all: reaching it needs a browser's Bluetooth handoff, so use a code instead.

**"A security key is connected but proton cannot open it."** On Linux, `/dev/hidraw*` belongs to root until a udev rule says otherwise, and no sign-in should need to be root. Every distribution ships the rules with its FIDO packages - `libfido2` on most, `yubikey-personalization` alongside it for YubiKeys. On NixOS:

```nix
services.udev.packages = [ pkgs.libfido2 ];
```

Unplug the key and plug it in again afterwards: the rules apply when the device appears.

**"This build cannot reach a security key on this machine."** A Windows build installed with `go install`. Windows hands out assertions through its own API, which the released binaries are built to reach and a `go install` build is not - the same difference that leaves it without the CAPTCHA helper. Install from a [release](installation.md), or sign in with `--totp`.

## Signing in on a headless machine

Only **login** can hit a CAPTCHA. Everything after it uses the saved session, so the fix is to obtain the session somewhere with a display and carry it over:

1. Run any command on a desktop machine with the same account, solving the CAPTCHA once.
2. Copy `~/.config/proton-cli/sessions/<profile>.json` to the headless machine, preserving mode `0600`.
3. Commands there use the existing session and skip login entirely.

Treat that file as a secret while copying it: it contains the session refresh token. See [Security and encryption](how-it-works.md).

macOS keeps it under `~/Library/Application Support/proton-cli/`, Windows under `%APPDATA%\proton-cli\`.

### Bringing your own CAPTCHA helper

`PROTON_HV_HELPER` names an executable to open the CAPTCHA with, instead of the one proton carries:

```bash
PROTON_HV_HELPER=/path/to/proton-hv proton account login
```

It is read before the embedded copy, so it works in a build that has none. Two uses:

- **A build with no helper in it** - anything from `go install` - gains one. `scripts/build-hv-helpers.sh` builds it from this repository.
- **A packaged proton** can ship the helper as a real file and give it the graphics environment a webview needs, rather than putting that environment on the CLI itself, where every editor and pager proton opens would inherit it.

The helper takes a URL and prints the solved token. A path that isn't there, isn't a file, or isn't executable is reported as such rather than as verification being unavailable.

## "Profile is not signed in"

```console
$ proton --profile work mail messages list
Error: Profile "work" is not signed in.
Try:   proton account login --profile work
```

Either nothing was ever signed in under that name, or the session was revoked or expired. Signing in again as the same account is harmless and idempotent, which is why an unattended job can simply run it first:

```bash
proton account login --user "$ACCOUNT" --password-file "$CRED"
```

## It asks for my password again

Three commands reach endpoints Proton guards behind an elevated session, and Proton re-authenticates against the password itself rather than accepting your session:

- `calendar settings calendars delete`
- `mail messages expire`
- `mail settings autoreply set`

They prompt, or take `--password-file` / `--password-stdin`. The key password sealed into your session cannot stand in: it is a one-way derivation.

Proton answers some unrelated refusals with the same code, so one of these may ask for your password and then be refused anyway, for a reason no password would have fixed.

## A change I just made doesn't show up

`list` reads Proton's server-side index, which is eventually consistent. A message you just sent may not appear for a few seconds, and one you just deleted may still be listed. The web client behaves the same way.

To confirm a change, run `get` on the ID the command printed rather than searching for the subject again. ([Why](design-notes.md#why-search-lags).)

## Everything started failing at once

Bulk commands page through Proton's API and respect its caps, but an account has limits above that - the sending quota on free accounts being the one most people meet. A run that suddenly fails with exit `5` after working fine is almost always a rate limit, not a bug.

Wait it out. A loop that acts on many things should sleep between iterations rather than going as fast as the API allows.

A 502 from Proton's edge, or a connection that drops, is waited out and retried automatically - for anything that only reads, and for signing in. Nothing that changes something is ever sent twice.

## "Work is a label, not a folder"

```console
$ proton mail messages move 5bH2mQxK --into Work
Error: "Work" is a label, not a folder - moving needs a folder.
Try:   to attach the label instead, use `label --label Work`.
       To see the folders, run `proton mail settings folders list`.
```

A message lives in exactly one folder and carries any number of labels. `move` changes the first, `label` adds to the second.

## A short ID doesn't resolve

Short IDs are remembered per machine, in `~/.config/proton-cli/idcache/<profile>.json`. One copied from another machine's output means nothing here. Run the matching `list` first, or use the full ID. ([More](language.md#short-ids).)

## A command hangs instead of failing

It doesn't - but a prompt on a terminal looks like a hang if you weren't expecting one. Only `account login` and the three elevated commands above ever ask a question, and only when standard input is a terminal. To make a missing credential an error instead:

```bash
proton account login --no-input
PROTON_NO_INPUT=1 proton account login
```

## Colours look wrong

proton asks your terminal for a colour by name and your terminal decides what it looks like. The exception is the `■` beside a label, folder, calendar or group: that hex is the value stored on Proton's side, and drawing it faithfully needs 24-bit colour. If your terminal takes it but does not advertise it, set `COLORTERM=truecolor`.

`--no-color` or `NO_COLOR` turns colour off entirely. ([Why it works this way](design-notes.md#why-colour-is-asked-for-by-name).)

## Still stuck

Run the command again with `--log-level debug` and open an [issue](https://github.com/roman-16/proton-cli/issues) with what it printed. Redact the IDs and addresses you would rather not publish - none of them are needed to reproduce a bug.

Please don't report a **security** issue in a public issue. [`SECURITY.md`](../SECURITY.md) has the private channels.
