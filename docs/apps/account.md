# Account

Signing in and out, account settings, and the sessions Proton holds for you.

Every command and flag is in the [`proton account` reference](../commands/account.md). For profiles, environment variables and where things are stored, see [Configuration](../configuration.md).

## Where you stand

```console
$ proton account get
Email:       you@proton.me
Name:        Roman
Storage:     128.4 GB of 500 GB (26%)
Max Upload:  5.0 GB
Profile:     default
Session:     valid
Unlocked:    yes
ID:          Kd91mQxT…
```

`Session: valid` and `Unlocked: yes` together mean this machine can act as the account right now.

## Signing in and out

```bash
proton account login
proton account logout
proton account logout --revoke     # also invalidate it at Proton
proton account logout --all        # every profile on this machine
```

`login` asks for whatever a flag has not already supplied, as long as it is running in a terminal. Without one, name the account with `--user` and hand the password over with `--password-file` or `--password-stdin` ([why not `--password`](../design-notes.md#why-a-password-is-never-a-flag-value)).

It also unlocks your keys, so your password is needed **once** per machine and not again. Signing in again as the same account changes nothing, so an unattended job can run it ahead of its real work and recover on its own from a session that expired.

An account in [two-password mode](https://proton.me/support/switch-two-password-mode) is asked for its second password once it has signed in - that is the one its keys are locked with - or reads it from `--second-password-file` / `--second-password-stdin`. Standard input has one reader, so at most one of the two secrets can come down a pipe.

Security keys are not supported: they need a browser. If your account uses one, add an authenticator app in Proton's settings and sign in with that code.

## Profiles

A profile is a named session slot on this machine, so a personal and a work account never mix.

```console
$ proton --profile work account login
✓ Signed in as you@company.com (profile "work").

$ proton account profiles list
PROFILE   EMAIL             UNLOCKED  SAVED             ACTIVE
────────  ────────────────  ────────  ────────────────  ──────
default   you@proton.me     yes       2026-04-15 14:31  ✓
work      you@company.com   yes       2026-04-15 15:02
```

`profiles list` works offline. `export PROTON_PROFILE=work` makes one the default for a shell.

## Sessions

Every session Proton holds for your account, across all your devices - the "Sessions" section of Proton's account settings.

```console
$ proton account sessions list
ID        CLIENT     CREATED           CURRENT
────────  ─────────  ────────────────  ───────
7Kd91mQx  web-mail   2026-04-15 14:31  ✓
3Ns8pT2v  ios-mail   2026-03-02 08:11
9xL4pQrT  web-drive  2026-01-20 19:44
```

```bash
proton account sessions revoke 3Ns8pT2v
proton account sessions revoke --others     # everything but this one
```

**If you lose a device, revoke its session.** That also makes the credentials saved on it useless, even to someone who already copied the file.

## Settings

```bash
proton account settings get     # the values now in effect
proton account settings list    # the keys you can change, with their values
proton account settings set locale de_AT
```

| Key | Values |
| --- | --- |
| `locale` | any text, e.g. `de_AT` |
| `date-format` | `locale`, `dd/mm/yyyy`, `mm/dd/yyyy`, `yyyy-mm-dd` |
| `time-format` | `locale`, `24h`, `12h` |
| `week-start` | `locale`, `monday` … `sunday` |
| `crash-reports` · `telemetry` | `off`, `on` |

Values can be given by name or by Proton's own number, and mistakes are caught before anything is sent.

`get` shows more than `set` can change. Proton Sentinel, two-factor state, whether the account is in two-password mode and recovery addresses are readable here but can only be changed at [account.proton.me](https://account.proton.me), along with your password, recovery secrets, billing and account deletion.

Mail, Calendar and Drive each have settings of their own - `proton mail settings`, and so on. Pass and Contacts have none.
