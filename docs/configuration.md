# Configuration

## Signing in

An account reaches proton one way:

```bash
proton account login
```

It asks for your email, password and two-factor code, attaches the account to a profile, and saves the session. Every later command acts as whichever profile it names.

Your password never leaves your machine: it derives the keys that decrypt your data locally. See [How it works](how-it-works.md).

### Handing over the password without a terminal

A password is read from a pipe or a file, never from a flag value: argv is readable by every user on the machine through `ps`, and it survives in shell history and in unit files.

```bash
# a pipe
printf '%s' "$PW" | proton account login --user alice@proton.me --password-stdin

# a file
proton account login --user alice@proton.me --password-file /run/secrets/proton
```

`account login` performs the exchange that attaches an account to a profile. The others reach an endpoint Proton guards behind an elevated session, which it grants only for another one - today `calendar settings calendars delete` and `mail settings autoreply set`:

```bash
printf '%s' "$PW" | proton calendar settings calendars delete Work --password-stdin
```

`--password-file` is usually the one to reach for on a machine, since systemd's `LoadCredential=`, Kubernetes secrets and Docker secrets all hand you a path already.

`--password-stdin` takes standard input for the password and nothing else, so it cannot be combined with a `-` argument that wants the same stream:

```console
$ printf '%s' "$PW" | proton --password-stdin mail messages send --body - ...
Error: --password-stdin and --body - both read standard input, which can only be read once.
Try:   pass the password with --password-file instead
```

Signing in again as the same account changes nothing, so an unattended job can run it ahead of its real work and recover on its own from a session that expired or was revoked:

```bash
proton account login --user "$ACCOUNT" --password-file "$CRED"
proton drive items upload backup.zst /backups
```

## Sessions

`account login` saves the session to `~/.config/proton-cli/sessions/<profile>.json` (mode `0600`) and reuses it, so later commands don't re-authenticate.

Signing in also unlocks your keys and seals the key password into that file, encrypted with a key that lives on Proton's side. **So your password is needed once per machine and not again** - reading and writing encrypted content afterwards asks for nothing.

Two exceptions:

- Proton guards a few destructive endpoints behind an elevated session, and asks for your password at the moment you hit one. Deleting a calendar is the one the CLI reaches today. It prompts, or reads `--password-file` / `--password-stdin`, and drops the elevation again immediately. The sealed key password cannot stand in: it is a one-way derivation, and Proton re-authenticates against the password itself.
- A session revoked or expired elsewhere means signing in again.

```bash
proton account logout             # forget the session here
proton account logout --revoke    # and invalidate it at Proton
```

Revoking is what makes a leaked copy of the file worthless: the sealed key password cannot be opened without the session. See [Security](../SECURITY.md).

## Profiles: more than one account

A profile is a name with an account behind it. Each keeps its own session file, so a personal and a work account never mix.

```bash
proton account login --profile work
proton --profile work mail messages list
```

To see what is signed in here - answered from disk, with no API call:

```bash
proton account profiles list
```

Or make a profile the default for the shell:

```bash
export PROTON_PROFILE=work
proton mail messages list
```

A profile nobody signed in acts as nobody, and says so before it reaches the network:

```console
$ proton --profile work mail messages list
Error: Profile "work" is not signed in.
Try:   proton account login --profile work
```

Pointing a profile at a different account is a fine thing to want; it just has to be said out loud, since the name means that account everywhere else:

```console
$ proton account login --profile work --user someone@else.com
Error: Profile "work" is signed in as alice@company.com.
Try:   proton account logout --profile work
```

## Environment variables

Eight, and none of them can name an account.

| Variable | Description |
| --- | --- |
| `PROTON_PROFILE` | Active profile (default: `default`) |
| `PROTON_API_URL` | API base URL (default: `https://mail.proton.me/api`) |
| `NO_COLOR` | Set to any value, even empty, to turn colored output off ([no-color.org](https://no-color.org)) |
| `COLORTERM` | `truecolor` or `24bit` if your terminal takes 24-bit color and does not advertise it; only affects how exactly a color swatch is drawn ([why](output.md#colour)) |
| `PROTON_NO_INPUT` | Set to any value, even empty, to never prompt; a missing credential becomes an error |
| `PROTON_LOG_LEVEL` | `debug`, `info`, `warn` or `error` |
| `PROTON_NO_UPDATE_CHECK` | Set to any value, even empty, to never look for a new release ([what that is](installation.md#the-update-notice)) |
| `PROTON_HV_HELPER` | An executable to open a CAPTCHA with, instead of the embedded one ([why](human-verification.md#bringing-your-own-helper)) |

## Files on disk

| Path | Contents |
| --- | --- |
| `~/.config/proton-cli/sessions/<profile>.json` | Session tokens and the encrypted key password (mode `0600`) |
| `~/.config/proton-cli/idcache/<profile>.json` | Short-ID lookup table (see [The language](language.md)) |
| `~/.config/proton-cli/update-check.json` | When proton last looked for a new release ([why](installation.md#the-update-notice)) |

Those paths are the Linux ones. macOS uses `~/Library/Application Support/proton-cli/` and Windows uses `%APPDATA%\proton-cli\`. Nothing else is written; there is no config file to maintain.

## Global flags

| Flag | Effect |
| --- | --- |
| `-p`, `--profile NAME` | Which profile to act as |
| `-o`, `--output text\|json\|yaml` | Output format (default `text`) |
| `-n`, `--dry-run` | Preview a mutation without applying it |
| `-y`, `--yes` | Answer confirmation prompts with yes; needed by a script that removes things ([why](language.md#when-it-asks-first)) |
| `-q`, `--quiet` | Suppress the non-essential stderr output |
| `--full-ids` | Don't shorten IDs in interactive output |
| `--no-color` | Turn colored output off (env: `NO_COLOR`) |
| `--log-level debug\|info\|warn\|error` | Logging verbosity (env: `PROTON_LOG_LEVEL`) |
| `--no-input` | Never prompt; a missing credential becomes an error (env: `PROTON_NO_INPUT`) |

Five flags have a single-letter form, and they are the five you type most: `-p`, `-o`, `-n`, `-q`, `-y`. They cluster, so `-qn` is a quiet dry run. The letters are global and no subcommand may take one, so `-p` is the profile everywhere and never something else.

`--api-url URL` points the CLI at a different API host. It works but is hidden from `--help`, because it is for developing proton rather than for using it.
