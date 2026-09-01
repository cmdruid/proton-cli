# proton account

Your Proton account, its settings and your session.

Every command under `proton account`, with the arguments and flags it takes. For these commands in use, see [the guide](../apps/account.md).

Holds `get`, `login`, `logout`, `profiles`, `sessions` and `settings`.

## `get`

Show the account, its storage and this machine's session.

```
proton account get
```

```bash
proton account get
proton account get --output json
```

## `login`

Sign in and save the session for this profile.

Signing in also unlocks your keys, so your password is needed once per machine and not again. Anything a flag has not set is asked for, as long as this is a terminal.

An account with a security key is asked to touch it. With an authenticator app enabled as well, an empty answer to the code prompt reaches for the key instead - so --totp is what an unattended job uses, a key being something only a person can answer.

An account in two-password mode is asked for its second password once it has signed in, because that is the secret its keys are locked with rather than the one that proves who it is. A one-password account is never asked for it.

Pass can be protected with an extra password of its own. It is not asked for here - the first `pass` command asks, and Proton then lets the session reach Pass for as long as it lives. --extra-password-file hands it over now instead, which is what a run with nobody to ask needs.

Proton may ask you to prove you are human. The page it wants is printed, and can be solved on any device - so a machine with no display signs in like any other. A run that cannot be asked anything says which page to solve and which token to repeat the command with.

Signing in again as the same account changes nothing, so an unattended job can run it first and recover from a session that expired.

```
proton account login
```

```bash
proton account login
proton account login --profile work
proton account login --user me@proton.me --password-file /run/secrets/proton
proton account login --user me@proton.me --password-stdin --totp 123456
proton account login --user me@proton.me --password-file /run/secrets/proton --second-password-file /run/secrets/proton-second
proton account login --user me@proton.me --password-file /run/secrets/proton --extra-password-file /run/secrets/proton-pass
```

| Flag | Description |
| --- | --- |
| `--extra-password-file string` | Read the Pass extra password from a file |
| `--extra-password-stdin` | Read the Pass extra password from stdin |
| `--password-file string` | Read the account password from a file |
| `--password-stdin` | Read the account password from stdin |
| `--second-password-file string` | Read the second password (two-password mode) from a file |
| `--second-password-stdin` | Read the second password (two-password mode) from stdin |
| `--totp string` | Two-factor code |
| `--user string` | Proton account email to sign in as |

## `logout`

Discard the saved session for this profile.

The sealed key password on disk is useless without the session, so removing the file is enough to make it unreadable. --revoke additionally invalidates the session at Proton, which is what signing out in a Proton app does.

```
proton account logout
```

```bash
proton account logout
proton account logout --revoke
proton account logout --all
```

| Flag | Description |
| --- | --- |
| `--all` | Act on everything in scope, rather than a subset |
| `--revoke` | Also invalidate the session at Proton |

## `profiles`

Accounts signed in on this machine.

Holds `delete` and `list`.

### `profiles delete`

Remove saved sessions by profile name.

```
proton account profiles delete REF...
```

```bash
proton account profiles delete work
```

### `profiles list`

List the profiles with a saved session.

```
proton account profiles list
```

```bash
proton account profiles list
```

## `sessions`

Sessions Proton holds for this account.

Holds `list` and `revoke`.

### `sessions list`

List every signed-in session.

```
proton account sessions list
```

```bash
proton account sessions list
```

### `sessions revoke`

Invalidate sessions at Proton.

A revoked session cannot decrypt the key password sealed into its saved file, so revoking is what makes a leaked session file worthless.

```
proton account sessions revoke [REF...]
```

```bash
proton account sessions revoke 5bH2mQxK
proton account sessions revoke --others
```

| Flag | Description |
| --- | --- |
| `--others` | Revoke every session except this one |

## `settings`

Account-wide preferences.

Holds `get`, `list` and `set`.

### `settings get`

Show the account settings now in effect.

```
proton account settings get
```

```bash
proton account settings get
```

### `settings list`

List the account settings that can be changed.

```
proton account settings list
```

```bash
proton account settings list
```

### `settings set`

Change one account setting.

```
proton account settings set KEY VALUE
```

```bash
proton account settings set locale de_AT
proton account settings set news off
```

---

Every command also takes the [flags that work everywhere](README.md#flags-that-work-on-every-command).
