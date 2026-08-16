# Account

Signing in and out, account settings, and the sessions Proton holds for you.

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

`login` asks for whatever a flag has not already supplied, as long as it is running in a terminal. Without one, name the account with `--user` and hand the password over with `--password-file` or `--password-stdin`. It also unlocks your keys, so your password is needed **once** per machine and not again - later commands read and write encrypted content without it.

Signing in again as the same account changes nothing, so an unattended job can run it ahead of its real work and recover on its own from a session that expired or was revoked.

Security keys are not supported: they need a browser. If your account uses one, add an authenticator app in Proton's settings and sign in with that code.

```bash
proton account login --no-input    # fail instead of asking
```

## Profiles

A profile is a named session slot on this machine. An account gets into one by signing in:

```console
$ proton --profile work account login
Email:     you@company.com
Password:
✓ Signed in as you@company.com (profile "work").

$ proton account profiles list
PROFILE   EMAIL             UNLOCKED  SAVED             ACTIVE
────────  ────────────────  ────────  ────────────────  ──────
default   you@proton.me     yes       2026-04-15 14:31  ✓
work      you@company.com   yes       2026-04-15 15:02

$ proton account profiles delete old
✓ Deleted profile "old".
```

`profiles list` works offline.

## Sessions

The sessions Proton holds for your account, across every device. This is the "Sessions" section of Proton's account settings.

```console
$ proton account sessions list
ID        CLIENT     CREATED           CURRENT
────────  ─────────  ────────────────  ───────
7Kd91mQx  web-mail   2026-04-15 14:31  ✓
3Ns8pT2v  ios-mail   2026-03-02 08:11
9xL4pQrT  web-drive  2026-01-20 19:44

3 sessions.
```

```bash
proton account sessions revoke 3Ns8pT2v
proton account sessions revoke --others     # everything but this one
```

Revoking a session also makes the credentials saved on that machine useless, even if someone already has a copy of the file. If you lose a device, revoke its session.

## Settings

```bash
proton account settings get     # the values now in effect
proton account settings list    # the keys you can change
proton account settings set locale de_AT
```

```console
$ proton account settings list
KEY            VALUES                                      PAGE                  DESCRIPTION
─────────────  ──────────────────────────────────────────  ────────────────────  ──────────────────────────────
date-format    locale, dd/mm/yyyy, mm/dd/yyyy, yyyy-mm-dd  Language and time     How dates are written
locale         any text                                    Language and time     Interface language, e.g. de_AT
time-format    locale, 24h, 12h                            Language and time     Clock format
week-start     locale, monday … sunday                     Language and time     First day of the week
crash-reports  off, on                                     Security and privacy  Send crash reports to Proton
telemetry      off, on                                     Security and privacy  Send anonymous usage data

6 settings.
```

Values can be given by name or by Proton's own number. Mistakes are caught before anything is sent:

```console
$ proton account settings set week-start funday
Error: week-start accepts: locale, monday, tuesday, wednesday, thursday, friday, saturday, sunday.

$ proton account settings set nope on
Error: There is no account setting called "nope".
Try:   proton account settings list
```

`get` shows more than `set` can change. Proton Sentinel, two-factor state and recovery addresses are readable here but can only be changed at [account.proton.me](https://account.proton.me), along with your password, recovery secrets, billing, and account deletion.

## Product settings

Mail, Calendar and Drive each have their own settings:

```bash
proton mail settings get
proton calendar settings get
proton drive settings get
```

Pass and Contacts have no settings of their own.
