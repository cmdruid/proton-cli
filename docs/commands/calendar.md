# Calendar

Calendars and events, encrypted with your calendar key and signed with your address key.

`proton calendar` is the calendar itself; the calendars you keep events in are managed under [`calendar settings`](#settings), matching where Proton puts them.

## Events

### List and read

```bash
proton calendar events list
proton calendar events list --calendar Work --start 2026-04-15 --end 2026-04-30
proton calendar events get CALENDAR_ID/EVENT_ID
proton calendar events get "Team sync"           # by title
```

Every calendar is included unless `--calendar` narrows it to one, by name or ID. `--start` and `--end` are the first and last **whole** days to include, read in your own time zone, and nothing outside them is listed. Without them the next 30 days are listed.

An event is on a day when it touches any part of it, so a query for one day inside a three-day event returns it. An event that merely ends at midnight belongs to the day before, and an all-day event belongs to the dates it names whatever zone you read it in.

A recurring event is listed on each day it happens, with a reference that names that occurrence:

```console
$ proton calendar events list --start 2026-04-20 --end 2026-04-27
ID                         DATE        TIME     DURATION  TITLE           LOCATION
─────────────────────────  ──────────  ───────  ────────  ──────────────  ────────
4f2a1b9c@2026-04-20T09:00  2026-04-20  09:00    15m       Standup         Meet
7bd3e011                   2026-04-21  all day  1d        Public holiday
4f2a1b9c@2026-04-22T10:30  2026-04-22  10:30    30m       Standup (long)  Meet
4f2a1b9c@2026-04-27T09:00  2026-04-27  09:00    15m       Standup         Meet
```

### Create

```bash
proton calendar events create --title Dentist --start 2026-04-16T14:00 --duration 1h
proton calendar events create --title Conference --start 2026-04-20 --all-day --duration 3d
proton calendar events create --calendar Work --title "Quarterly sync" --start 2026-04-16T14:00 --duration 90m --location "Vienna HQ" --description "Numbers and roadmap"
```

Recurrence and reminders:

```bash
proton calendar events create --title Standup --start 2026-04-16T09:00 --duration 15m --rrule "FREQ=WEEKLY;COUNT=10" --remind 15m --remind 1h
```

`--rrule` takes an iCal recurrence rule; `--remind` is repeatable.

`--all-day` makes an event with no time of day, which is measured in days: it lasts one day unless `--duration` says otherwise, and `--duration 3d` covers three. Such an event ends at the midnight after its last day, which is how iCalendar and every other calendar client write it.

`--zone` anchors the event to an IANA time zone, defaulting to your system zone. It matters for a recurring event: a series anchored to `Europe/Vienna` stays at 09:00 when the clocks change, where one stored as a plain UTC instant would slide to 08:00.

```bash
proton calendar events create --title Standup --start 2026-04-16T09:00 --duration 15m \
  --rrule "FREQ=WEEKLY" --zone Europe/Vienna
```

Attendees:

```bash
proton calendar events create --title Review --start 2026-04-16T14:00 --attendee alice@proton.me --attendee bob@example.com
```

Proton users are added directly; external addresses get an emailed invitation.

### Update, respond, delete

```bash
proton calendar events update CALENDAR_ID/EVENT_ID --title "New title"
proton calendar events update CALENDAR_ID/EVENT_ID --start 2026-04-17T10:00 --duration 2h
proton calendar events update CALENDAR_ID/EVENT_ID --rrule "FREQ=WEEKLY;BYDAY=MO,TH"
proton calendar events update CALENDAR_ID/EVENT_ID --remind 30m --remind 1h
proton calendar events update CALENDAR_ID/EVENT_ID --no-remind
proton calendar events respond CALENDAR_ID/EVENT_ID --status accept
proton calendar events respond "Team sync" --status decline    # emails the organizer
proton calendar events delete CALENDAR_ID/EVENT_ID
proton calendar events delete "Dentist"
```

Anything you do not mention is left alone, including the reminders, the recurrence and the occurrences you have cancelled.

`--status` is `accept`, `tentative`, or `decline`, and applies to the whole invitation rather than to one occurrence.

## Recurring events

The reference says which occurrences a change reaches, and `--future` widens one occurrence to it and everything after.

| Command | Changes |
| --- | --- |
| `update 4f2a1b9c@2026-04-22T09:00 …` | that occurrence only |
| `update 4f2a1b9c@2026-04-22T09:00 --future …` | that occurrence and every later one |
| `update 4f2a1b9c …` | the whole series |
| `delete 4f2a1b9c@2026-04-22T09:00` | that occurrence only |
| `delete 4f2a1b9c@2026-04-22T09:00 --future` | that occurrence and every later one |
| `delete 4f2a1b9c` | the whole series |

```bash
# move one standup, leaving the series alone
proton calendar events update 4f2a1b9c@2026-04-22T09:00 --start 2026-04-22T10:30 --duration 30m

# cancel one standup
proton calendar events delete 4f2a1b9c@2026-04-22T09:00

# from May the 4th on, it moves half an hour later
proton calendar events update 4f2a1b9c@2026-05-04T09:00 --start 2026-05-04T09:30 --future

# end the series there
proton calendar events delete 4f2a1b9c@2026-05-04T09:00 --future
```

Deleting a series removes every occurrence, so it says how many and shows them first:

```console
$ proton calendar events delete 4f2a1b9c --dry-run
Dry run - would delete 12 events:

ID                         DATE        TIME   DURATION  TITLE    LOCATION
─────────────────────────  ──────────  ─────  ────────  ───────  ────────
4f2a1b9c@2026-04-06T09:00  2026-04-06  09:00  15m       Standup  Meet
…
```

`--future` on the first occurrence is refused, because nothing would be left: delete or update the series instead.

## Time formats

| Flag | Accepts |
| --- | --- |
| `--start` | `2026-04-16T14:00`, `2026-04-16 14:00`, `2026-04-16`, or full RFC 3339, in your system timezone |
| `--duration` | `15m`, `90m`, `1h`, `2h30m` |
| `--remind` | `15m`, `1h`, `1d` (repeatable) |
| `--start` / `--end` on `list` | `YYYY-MM-DD`, both days included |
| `--zone` | an IANA zone name, e.g. `Europe/Vienna` |
| an occurrence in a `REF` | the occurrence's own start, as `list` printed it |

## Settings

One subcommand per page of Proton's calendar settings.

```bash
proton calendar settings          # time zones, layout, invitations
proton calendar settings set      # the writable keys
```

```bash
proton calendar settings set view week
proton calendar settings set primary-timezone Europe/Vienna
proton calendar settings set week-numbers on
proton calendar settings set auto-import-invite on
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
proton calendar settings calendars list
proton calendar settings calendars create --name Work --color "#8080FF"
proton calendar settings calendars update CALENDAR_ID --name Personal --color "#DB60D6"
proton calendar settings calendars delete Work        # by name, or by calendar ID
```

Colors have to be Proton accent colors; an invalid value prints the allowed list. Deleting a calendar is a password-scoped operation, so it asks for your password even when a session already exists. With no terminal to ask, it takes `--password-file` or `--password-stdin`.
