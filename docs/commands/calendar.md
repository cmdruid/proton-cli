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
proton calendar events create --title Dentist --start 2026-04-16T14:00 --end 2026-04-16T15:00
proton calendar events create --title Conference --start 2026-04-20 --all-day --duration 3d
proton calendar events create --calendar Work --title "Quarterly sync" --start 2026-04-16T14:00 --duration 90m --location "Vienna HQ" --description "Numbers and roadmap"
```

Recurrence and reminders:

```bash
proton calendar events create --title Standup --start 2026-04-16T09:00 --duration 15m --rrule "FREQ=WEEKLY;COUNT=10" --remind 15m --remind 1h
proton calendar events create --title Renewal --start 2026-09-01T09:00 --remind 1d:email
```

`--rrule` takes an iCal recurrence rule.

`--remind` is repeatable and takes a duration before the start. A bare duration is a device notification; add `:email` for an emailed reminder, as Proton's own composer offers both. A listing prints them back in the same spelling.

How long an event lasts is said **once**, with either `--end` or `--duration`. Both together is refused, and so is either without `--start`.

`--status` says whether the event is going ahead: `confirmed` (the default), `tentative` or `cancelled`. Cancelling this way keeps the event and its history, which is what the web client does and what `delete` does not.

`--attendee` marks someone optional the same way: `--attendee jane@example.com:optional`. A bare address is required, which is what inviting someone ordinarily means.

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
proton calendar events respond CALENDAR_ID/EVENT_ID --answer accept
proton calendar events respond "Team sync" --answer decline    # emails the organizer
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

## Export

```bash
proton calendar events export --start 2026-01-01 --end 2026-12-31 --output year.ics
proton calendar events export --calendar Work --output -          # to stdout
```

Writes an `.ics` file, the format every other calendar reads and the one Proton's own settings page offers. A recurring series is written **once** with its rule, not expanded into its occurrences, so another client reads it back as the same series. Reminders travel as `VALARM` components, and a cancelled event stays cancelled.

An event whose content cannot be decrypted is left out rather than written as a stub, because a file is something another client will trust.

## Import

```bash
proton calendar events import holidays.ics
proton calendar events import --calendar Work team.ics
curl -s https://example.com/team.ics | proton calendar events import -
```

**An import is addressed by UID.** An event carries the UID of the event it is, so reading a file back changes that event rather than making a second one: export, edit the file, import, and the calendar says what the file says. The replacement is a new event with a new ID, so a reference held from before an import no longer resolves. `--dry-run` lists what the file holds before any of it lands.

**Participants are left out.** An imported event is a record of something, not an invitation being reissued, and writing the guests back would make your account the organizer of a meeting it did not call - which, for an event with external addresses, means email going out.

An event with no start time is skipped and named; the rest still land.

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

Each calendar carries its own defaults for the events made in it:

```bash
proton calendar settings calendars get Work
proton calendar settings calendars update Work --default-duration 30m --remind 15m
proton calendar settings calendars update Personal --busy off
```

`--busy` says whether events there make you look busy to people who check your availability. `--remind-all-day` sets the default for events with no time of day, and `--no-remind` gives new events none.


```bash
proton calendar settings calendars list
proton calendar settings calendars create --name Work --color "#8080FF"
proton calendar settings calendars update CALENDAR_ID --name Personal --color "#DB60D6"
proton calendar settings calendars delete Work        # by name, or by calendar ID
```

Colors have to be Proton accent colors; an invalid value prints the allowed list. Deleting a calendar is a password-scoped operation, so it asks for your password even when a session already exists. With no terminal to ask, it takes `--password-file` or `--password-stdin`.

### Sharing a calendar

```bash
proton calendar settings calendars share add Work jane@proton.me
proton calendar settings calendars share add Work jane@proton.me --edit
proton calendar settings calendars share list Work
proton calendar settings calendars share remove Work jane@proton.me
```

A calendar is opened by a passphrase, and every member holds that passphrase encrypted to their own key. So sharing is not a permission Proton grants - it is handing somebody the key that opens the calendar, **encrypted so only they can read it and signed so they can tell it came from you**. Proton passes it along without being able to read it.

That is also why it only works with another Proton account: an address Proton holds no keys for has nothing to encrypt to, and the command says so rather than sending an invitation nobody can take.

They see nothing until they accept, and until then `share list` shows them as `pending`. `share remove` works either way - an unanswered invitation is withdrawn, a membership somebody is using is ended.

### A calendar somebody gave you

```bash
proton calendar invitations list
proton calendar invitations accept Work
proton calendar invitations decline Work
```

Until you accept, you see the calendar's name and who sent it and nothing that is on it. Accepting opens the key the invitation carries and signs it back with the address it was sent to, which is how Proton knows the offer reached somebody who could read it. Afterwards the calendar reads like any other of yours.

### Subscribing to a calendar published elsewhere

```bash
proton calendar settings calendars create --name Timetable --url https://example.com/team.ics
```

`--url` takes the address of an `.ics` file - a timetable, a team's shared calendar, a holiday feed. Proton fetches it on a schedule and fills the calendar from it, so the events are **read-only**: they belong to whoever publishes them. A listing says which calendars are which, under `KIND`.

Proton is asked whether it can read the address before the calendar is made, so a wrong one is refused rather than leaving you with a calendar that never fills - and the refusal carries Proton's own account of it, down to the HTTP status.
