# Settings

Settings are scoped the way Proton scopes them: `settings` is your account, and each product carries its own tree.

| Command | Proton settings section |
| --- | --- |
| `proton-cli settings` | Account |
| [`proton-cli mail settings`](mail.md#settings) | Mail |
| [`proton-cli calendar settings`](calendar.md#settings) | Calendar |
| [`proton-cli drive settings`](drive.md#settings) | Drive |

A bare settings command shows the current state; `set` changes one value.

## Account settings

```bash
proton-cli settings                    # locale, recovery, privacy
proton-cli settings --output json
proton-cli settings set                # the writable keys, grouped by page
```

```bash
proton-cli settings set locale de_AT
proton-cli settings set date-format yyyy-mm-dd
proton-cli settings set time-format 24h
proton-cli settings set week-start monday
proton-cli settings set telemetry off
proton-cli settings set crash-reports off
```

| Key | Values | Page |
| --- | --- | --- |
| `locale` | an interface language, e.g. `en_US`, `de_AT` | Language and time |
| `date-format` | `locale`, `dd/mm/yyyy`, `mm/dd/yyyy`, `yyyy-mm-dd` | Language and time |
| `time-format` | `locale`, `24h`, `12h` | Language and time |
| `week-start` | `locale`, `monday` … `sunday` | Language and time |
| `telemetry` | `off`, `on` | Security and privacy |
| `crash-reports` | `off`, `on` | Security and privacy |

Your password, two-factor setup, recovery secrets, account deletion and billing are deliberately absent: they are not things a script should change in one line. Proton Sentinel and Dark Web Monitoring are read-only here for the same reason — `settings` reports Sentinel's state, but turning a security feature off is a decision for the web client.

## Setting values are checked before they are sent

Every key declares the values it accepts, so a mistake is caught locally with the whole domain spelled out rather than bounced back by the API:

```console
$ proton-cli settings set week-start funday
Error: week-start accepts: locale, monday, tuesday, wednesday, thursday, friday, saturday, sunday
```

Named values exist so you never have to remember Proton's numbers, and the numbers still work if you prefer them. Shell completion offers both keys and values.

## Product settings

```bash
proton-cli mail settings              # and mail settings set / addresses / labels / filters / autoreply
proton-cli calendar settings          # and calendar settings set / calendars
proton-cli drive settings             # and drive settings set
```

Pass has no settings tree: its account-settings pages are app downloads and organisation-only reports. Contacts has none either — contacts are managed inside Mail.
