# Calendar

Calendars and events, encrypted with your calendar key and signed with your address key.

`proton-cli calendar` is the calendar itself; the calendars you keep events in are managed under [`calendar settings`](#settings), matching where Proton puts them.

## Events

### List and read

```bash
proton-cli calendar events list
proton-cli calendar events list --calendar Work --start 2026-04-15 --end 2026-04-30
proton-cli calendar events get CALENDAR_ID/EVENT_ID
proton-cli calendar events get "Team sync"           # by title
```

`--calendar` takes a calendar name or ID.

### Create

```bash
proton-cli calendar events create --title Dentist --start 2026-04-16T14:00 --duration 1h
proton-cli calendar events create --title Conference --start 2026-04-20 --all-day
proton-cli calendar events create --calendar Work --title "Quarterly sync" --start 2026-04-16T14:00 --duration 90m --location "Vienna HQ" --description "Numbers and roadmap"
```

Recurrence and reminders:

```bash
proton-cli calendar events create --title Standup --start 2026-04-16T09:00 --duration 15m --rrule "FREQ=WEEKLY;COUNT=10" --remind 15m --remind 1h
```

`--rrule` takes an iCal recurrence rule; `--remind` is repeatable.

Attendees:

```bash
proton-cli calendar events create --title Review --start 2026-04-16T14:00 --attendee alice@proton.me --attendee bob@example.com
```

Proton users are added directly; external addresses get an emailed invitation.

### Update, respond, delete

```bash
proton-cli calendar events update CALENDAR_ID/EVENT_ID --title "New title"
proton-cli calendar events update CALENDAR_ID/EVENT_ID --start 2026-04-17T10:00 --duration 2h
proton-cli calendar events respond CALENDAR_ID/EVENT_ID --status accept
proton-cli calendar events respond "Team sync" --status decline    # emails the organizer
proton-cli calendar events delete CALENDAR_ID/EVENT_ID
proton-cli calendar events delete "Dentist"
```

`--status` is `accept`, `tentative`, or `decline`.

## Time formats

| Flag | Accepts |
| --- | --- |
| `--start` | `2026-04-16T14:00`, `2026-04-16 14:00`, `2026-04-16`, or full RFC 3339, in your system timezone |
| `--duration` | `15m`, `90m`, `1h`, `2h30m` |
| `--remind` | `15m`, `1h`, `1d` (repeatable) |
| `--start` / `--end` on `list` | `YYYY-MM-DD` |

## Settings

One subcommand per page of Proton's calendar settings.

```bash
proton-cli calendar settings          # time zones, layout, invitations
proton-cli calendar settings set      # the writable keys
```

```bash
proton-cli calendar settings set view week
proton-cli calendar settings set primary-timezone Europe/Vienna
proton-cli calendar settings set week-numbers on
proton-cli calendar settings set auto-import-invite on
```

| Key | Values |
| --- | --- |
| `view` | `day`, `week`, `month`, `year`, `planning` |
| `week-numbers` | `off`, `on` |
| `primary-timezone` | an IANA zone, e.g. `Europe/Vienna` |
| `auto-detect-timezone` | `off`, `on` |
| `secondary-timezone` | an IANA zone |
| `show-secondary-timezone` | `off`, `on` |
| `auto-import-invite` | `off`, `on` |
| `invite-locale` | a language, e.g. `en_US` |
| `default-calendar` | a calendar ID |

### Calendars

```bash
proton-cli calendar settings calendars list
proton-cli calendar settings calendars create --name Work --color "#8080FF"
proton-cli calendar settings calendars update CALENDAR_ID --name Personal --color "#DB60D6"
proton-cli calendar settings calendars delete Work        # by name, or by calendar ID
```

Colors have to be Proton accent colors; an invalid value prints the allowed list. Deleting a calendar is a password-scoped operation, so `PROTON_PASSWORD` has to be set even when a session already exists.
