# Configuration

## Credentials

proton-cli reads your credentials from the environment:

```bash
export PROTON_USER=alice@proton.me
export PROTON_PASSWORD=your-password
export PROTON_TOTP=123456      # only if two-factor authentication is enabled
```

Your password never leaves your machine: it derives the keys that decrypt your data locally. See [How it works](how-it-works.md).

Put those lines in a password-manager-backed shell snippet rather than your shell profile if you can, for example:

```bash
export PROTON_PASSWORD=$(pass show proton/password)
```

Every variable also has a flag equivalent (`--user`, `--password`, `--totp`), which is handy for one-off overrides but leaves credentials in your shell history.

## Sessions

`proton-cli account login` saves the session to `~/.config/proton-cli/sessions/<profile>.json` (mode `0600`) and reuses it, so later commands don't re-authenticate.

Signing in also unlocks your keys and seals the key password into that file, encrypted with a key that lives on Proton's side. **So your password is needed once per machine and not again** - reading and writing encrypted content afterwards asks for nothing.

Two exceptions:

- Proton guards a few destructive endpoints behind an elevated session, and asks for your password at the moment you hit one. Deleting a calendar is the one the CLI reaches today. It prompts, or reads `PROTON_PASSWORD`, and drops the elevation again immediately.
- A session revoked or expired elsewhere means signing in again.

```bash
proton-cli account logout             # forget the session here
proton-cli account logout --revoke    # and invalidate it at Proton
```

Revoking is what makes a leaked copy of the file worthless: the sealed key password cannot be opened without the session. See [Security](../SECURITY.md).

## Environment variables

| Variable | Description |
| --- | --- |
| `PROTON_USER` | Account email |
| `PROTON_PASSWORD` | Account password |
| `PROTON_TOTP` | Current TOTP code, when 2FA is on |
| `PROTON_PROFILE` | Active profile (default: `default`) |
| `PROTON_API_URL` | API base URL (default: `https://mail.proton.me/api`) |
| `PROTON_APP_VERSION` | App version header (default: `Other`) |
| `NO_COLOR` | Set to any value, even empty, to turn colored output off ([no-color.org](https://no-color.org)) |
| `PROTON_NO_INPUT` | Set to any value, even empty, to never prompt; a missing credential becomes an error |
| `PROTON_LOG_LEVEL` | `debug`, `info`, `warn` or `error` |

## Profiles: more than one account

A profile is just a name. Each one keeps its own session file and its own set of environment variables, so a personal and a work account never mix.

```bash
proton-cli --profile work account login
proton-cli --profile work mail messages list
```

With an interactive login a second account needs no variables at all. To see what is signed in here:

```bash
proton-cli account profiles list
```

Or make a profile the default for the shell:

```bash
export PROTON_PROFILE=work
proton-cli mail messages list
```

Every setting except `NO_COLOR`, `PROTON_NO_INPUT` and `PROTON_LOG_LEVEL` can be scoped to a profile by putting the profile name into the variable: `PROTON_<PROFILE>_USER`, `PROTON_<PROFILE>_PASSWORD`, and so on. The profile name is upper-cased with anything non-alphanumeric replaced by `_` (`work` → `WORK`, `my-work` → `MY_WORK`).

```bash
export PROTON_USER=alice@proton.me            # default profile
export PROTON_PASSWORD=personal-password

export PROTON_WORK_USER=alice@company.com     # work profile
export PROTON_WORK_PASSWORD=work-password

proton-cli mail messages list                 # personal
proton-cli --profile work mail messages list  # work
```

### Resolution order

For every setting, the most specific source wins:

1. the flag (`--user`, `--password`, …)
2. the profile-scoped variable (`PROTON_WORK_USER`)
3. the plain variable (`PROTON_USER`)

That means shared values can live in `PROTON_*` while only the differences need a profile-scoped variable.

## Files on disk

| Path | Contents |
| --- | --- |
| `~/.config/proton-cli/sessions/<profile>.json` | Session tokens and the encrypted key password (mode `0600`) |
| `~/.config/proton-cli/idcache/<profile>.json` | Short-ID lookup table (see [The language](language.md)) |

Those paths are the Linux ones. macOS uses `~/Library/Application Support/proton-cli/` and Windows uses `%APPDATA%\proton-cli\`. Nothing else is written; there is no config file to maintain.

## Global flags

| Flag | Effect |
| --- | --- |
| `--output text\|json\|yaml` | Output format (default `text`) |
| `--profile NAME` | Which profile to use |
| `--dry-run` | Preview a mutation without applying it |
| `--yes` | Answer confirmation prompts with yes; needed by a script that removes things ([why](language.md#when-it-asks-first)) |
| `--full-ids` | Don't shorten IDs in interactive output |
| `--no-color` | Turn colored output off (env: `NO_COLOR`) |
| `--quiet` | Suppress the non-essential stderr output |
| `--log-level debug\|info\|warn\|error` | Logging verbosity (env: `PROTON_LOG_LEVEL`) |
| `--no-input` | Never prompt; a missing credential becomes an error (env: `PROTON_NO_INPUT`) |
| `--api-url URL` | Point at a different API host |
| `--app-version STRING` | Override the app version header |
