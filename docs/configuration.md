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

After the first successful login, the session is saved to `~/.config/proton-cli/sessions/<profile>.json` (mode `0600`) and reused, so later commands don't re-authenticate.

`PROTON_PASSWORD` is still needed for anything that touches encrypted content, because unlocking your PGP keys requires it. `PROTON_TOTP` is only used during a fresh login.

Delete the session file to force a new login. See [Security](../SECURITY.md) for exactly what's inside it.

## Environment variables

| Variable | Description |
| --- | --- |
| `PROTON_USER` | Account email |
| `PROTON_PASSWORD` | Account password |
| `PROTON_TOTP` | Current TOTP code, when 2FA is on |
| `PROTON_PROFILE` | Active profile (default: `default`) |
| `PROTON_API_URL` | API base URL (default: `https://mail.proton.me/api`) |
| `PROTON_APP_VERSION` | App version header (default: `Other`) |
| `NO_COLOR` | Set to any value to turn colored output off ([no-color.org](https://no-color.org)) |

## Profiles: more than one account

A profile is just a name. Each one keeps its own session file and its own set of environment variables, so a personal and a work account never mix.

```bash
proton-cli --profile work mail messages list
```

Or make it the default for the shell:

```bash
export PROTON_PROFILE=work
proton-cli mail messages list
```

Every setting can be scoped to a profile by putting the profile name into the variable: `PROTON_<PROFILE>_USER`, `PROTON_<PROFILE>_PASSWORD`, and so on. The profile name is upper-cased with anything non-alphanumeric replaced by `_` (`work` → `WORK`, `my-work` → `MY_WORK`).

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
| `~/.config/proton-cli/idcache/<profile>.json` | Short-ID lookup table (see [Concepts](concepts.md)) |

Those paths are the Linux ones. macOS uses `~/Library/Application Support/proton-cli/` and Windows uses `%APPDATA%\proton-cli\`. Nothing else is written; there is no config file to maintain.

## Global flags

| Flag | Effect |
| --- | --- |
| `--output text\|json\|yaml` | Output format (default `text`) |
| `--profile NAME` | Which profile to use |
| `--dry-run` | Preview a mutation without applying it |
| `--full-ids` | Don't shorten IDs in interactive output |
| `--no-color` | Turn colored output off |
| `--quiet` | Suppress the non-essential stderr output |
| `--verbose` | Debug logging |
| `--log-level debug\|info\|warn\|error` | Finer-grained logging |
| `--api-url URL` | Point at a different API host |
| `--app-version STRING` | Override the app version header |
