package calendar

import (
	"fmt"
	"time"

	"github.com/roman-16/proton-cli/internal/cli/kit"
	"github.com/roman-16/proton-cli/internal/ical"
	calsvc "github.com/roman-16/proton-cli/internal/service/calendar"
	"github.com/roman-16/proton-cli/internal/ui"
	"github.com/roman-16/proton-cli/internal/units"
	"github.com/spf13/cobra"
)

func eventsCmd() *cobra.Command {
	c := &cobra.Command{Use: "events", Short: "Events in your calendars"}
	c.AddCommand(eventsListCmd(), eventsGetCmd(), eventsCreateCmd(), eventsUpdateCmd(),
		eventsRespondCmd(), eventsDeleteCmd())
	return c
}

// An event needs two IDs to address it, because Proton stores it inside a
// calendar. Writing them as one slash-separated token keeps every command to a
// single REF, and is safe because Proton's IDs are base64url and never contain a
// slash.
func eventRef(e calsvc.Event) string { return kit.JoinPair(e.CalendarID, e.ID) }

// resolveEvent turns a reference into the pair the service needs. A title still
// works, and still resolves across every calendar.
func resolveEvent(c *kit.Invocation, ref string) (calendarID, eventID string, err error) {
	u, err := c.App.Unlock(c.Ctx)
	if err != nil {
		return "", "", err
	}
	if first, second := kit.Pair(ref); first != "" {
		return first, second, nil
	}
	return c.App.Calendar.ResolveEvent(c.Ctx, u, []string{ref})
}

func eventColumns() []ui.Column[calsvc.Event] {
	return []ui.Column[calsvc.Event]{
		{Header: "ID", ID: true, Cell: eventRef},
		{Header: "DATE", Cell: func(e calsvc.Event) string { return e.Start.Local().Format("2006-01-02") }},
		{Header: "TIME", Cell: func(e calsvc.Event) string {
			if e.AllDay {
				return "all day"
			}
			return e.Start.Local().Format("15:04")
		}},
		{Header: "DURATION", Right: true, Cell: func(e calsvc.Event) string {
			return units.Duration(e.End.Sub(e.Start))
		}},
		{Header: "TITLE", Flex: true, Cell: func(e calsvc.Event) string { return e.Title }},
		{Header: "LOCATION", Flex: true, Cell: func(e calsvc.Event) string { return e.Location }},
	}
}

func eventsListCmd() *cobra.Command {
	var calendar, start, end string
	c := &cobra.Command{
		Use:   "list",
		Short: "List events in a date range",
		Args:  cobra.NoArgs,
		RunE: kit.Run([]kit.Step{kit.StepAuth, kit.StepUnlock}, func(c *kit.Invocation) error {
			calID, err := resolveCalendar(c, calendar)
			if err != nil {
				return err
			}
			from, to := calsvc.DefaultRange()
			if start != "" {
				t, err := time.Parse("2006-01-02", start)
				if err != nil {
					return kit.Fail("--start expects YYYY-MM-DD.")
				}
				from = t
			}
			if end != "" {
				t, err := time.Parse("2006-01-02", end)
				if err != nil {
					return kit.Fail("--end expects YYYY-MM-DD.")
				}
				to = t
			}
			events, err := c.App.Calendar.EventsList(c.Ctx, c.U, calID, from, to)
			if err != nil {
				return err
			}
			return kit.List(c, ui.TableSpec[calsvc.Event]{
				Noun: "events", Columns: eventColumns(),
				Total: ui.Unknown, Page: ui.Unpaged,
			}, events, func(e calsvc.Event) []string { return []string{e.CalendarID, e.ID} })
		}),
	}
	c.Flags().StringVar(&calendar, "calendar", "", "Which calendar, by name or ID (default: your first)")
	c.Flags().StringVar(&start, "start", "", "First day to include (YYYY-MM-DD)")
	c.Flags().StringVar(&end, "end", "", "Last day to include (YYYY-MM-DD)")
	return c
}

func eventsGetCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "get REF",
		Short: "Show one event, decrypted",
		Args:  cobra.ExactArgs(1),
		RunE: kit.Run([]kit.Step{kit.StepAuth, kit.StepExpand, kit.StepUnlock}, func(c *kit.Invocation) error {
			calID, eventID, err := resolveEvent(c, c.Args[0])
			if err != nil {
				return err
			}
			ev, err := c.App.Calendar.EventGet(c.Ctx, c.U, calID, eventID)
			if err != nil {
				return err
			}
			name, err := calendarName(c, ev.CalendarID)
			if err != nil {
				return err
			}
			when := ev.Start.Local().Format("2006-01-02 15:04")
			until := ev.End.Local().Format("2006-01-02 15:04")
			if ev.AllDay {
				when = ev.Start.Local().Format("2006-01-02") + " (all day)"
				until = ""
			}
			return kit.Show(c, ui.RecordSpec{
				Object: ev,
				Fields: []ui.Field{
					{Label: "Title", Value: ev.Title},
					{Label: "Start", Value: when},
					{Label: "End", Value: until},
					{Label: "Duration", Value: units.Duration(ev.End.Sub(ev.Start))},
					{Label: "Location", Value: ev.Location},
					{Label: "Description", Value: ev.Description},
					{Label: "Recurrence", Value: ev.RRule},
					{Label: "Calendar", Value: name},
					{Label: "Signature", Value: string(ev.Signature), Always: true},
					{Label: "ID", Value: eventRef(*ev), ID: true},
				},
			})
		}),
	}
}

// details are the fields an event carries. create and update share them, so the
// two commands cannot disagree about what an event is.
type details struct {
	title, location, description string
	start, duration, rrule       string
	allDay                       bool
	reminders, attendees         []string
}

func (d *details) register(c *cobra.Command, verb string) {
	f := c.Flags()
	f.StringVar(&d.title, "title", "", verb+" the title")
	f.StringVar(&d.start, "start", "", verb+" the start (RFC 3339, or YYYY-MM-DDTHH:MM)")
	f.StringVar(&d.duration, "duration", "", verb+" how long it lasts (e.g. 15m, 1h, 2h30m)")
	f.StringVar(&d.location, "location", "", verb+" where it is")
	f.StringVar(&d.description, "description", "", verb+" the description")
}

func eventsCreateCmd() *cobra.Command {
	var d details
	var calendar string
	c := &cobra.Command{
		Use:   "create",
		Short: "Create an event",
		Args:  cobra.NoArgs,
		RunE: kit.Run([]kit.Step{kit.StepAuth, kit.StepUnlock}, func(c *kit.Invocation) error {
			if d.title == "" || d.start == "" {
				return kit.Fail("An event needs a title and a start.").
					Hint(`--title Dentist --start 2026-04-16T14:00`)
			}
			calID, err := resolveCalendar(c, calendar)
			if err != nil {
				return err
			}
			start, err := ical.ParseTime(d.start)
			if err != nil {
				return kit.Fail("--start: %v", err)
			}
			dur := time.Hour
			if d.duration != "" {
				parsed, err := time.ParseDuration(d.duration)
				if err != nil {
					return kit.Fail("--duration: %v", err)
				}
				dur = parsed
			}
			var res *calsvc.EventResult
			if err := kit.Create(c, ui.ResultSpec{
				Action: ui.Created, Kind: "events", Name: d.title,
			}, func() (string, error) {
				var err error
				res, err = c.App.Calendar.EventCreate(c.Ctx, c.U, calID, calsvc.EventInput{
					Title: d.title, Location: d.location, Description: d.description,
					Start: start, End: start.Add(dur), AllDay: d.allDay,
					RRule: d.rrule, Reminders: d.reminders, Attendees: d.attendees,
				})
				if err != nil {
					return "", err
				}
				return kit.JoinPair(calID, res.ID), nil
			}); err != nil {
				return err
			}
			// External attendees are told by email, since they have no Proton
			// calendar to add the event to. A failure here does not undo the event.
			if res != nil && res.Invite != nil {
				body := fmt.Sprintf("You have been invited to %q.\n\nThe calendar invitation is attached.", d.title)
				if err := sendICS(c, c.U, res.Invite.Recipients, res.Invite.Subject, body, res.Invite.ICS, "REQUEST"); err != nil {
					c.Note("The event was created, but the invitation email to %s failed: %v",
						ui.Quantity(len(res.Invite.Recipients), "attendees"), err)
				}
			}
			return nil
		}),
	}
	d.register(c, "Set")
	c.Flags().StringVar(&calendar, "calendar", "", "Which calendar, by name or ID (default: your first)")
	c.Flags().BoolVar(&d.allDay, "all-day", false, "An event with no time of day")
	c.Flags().StringVar(&d.rrule, "rrule", "", "Recurrence rule, e.g. FREQ=WEEKLY;COUNT=10")
	c.Flags().StringArrayVar(&d.reminders, "remind", nil, "Remind this long before the start (repeatable)")
	c.Flags().StringArrayVar(&d.attendees, "attendee", nil,
		"Invite someone; Proton users are added directly, others are emailed (repeatable)")
	return c
}

func eventsUpdateCmd() *cobra.Command {
	var d details
	c := &cobra.Command{
		Use:   "update REF",
		Short: "Change an event's title, time, location or description",
		Args:  cobra.ExactArgs(1),
		RunE: kit.Run([]kit.Step{kit.StepAuth, kit.StepExpand, kit.StepUnlock}, func(c *kit.Invocation) error {
			calID, eventID, err := resolveEvent(c, c.Args[0])
			if err != nil {
				return err
			}
			var start, end time.Time
			if d.start != "" {
				t, err := ical.ParseTime(d.start)
				if err != nil {
					return kit.Fail("--start: %v", err)
				}
				start = t
				if d.duration != "" {
					dur, err := time.ParseDuration(d.duration)
					if err != nil {
						return kit.Fail("--duration: %v", err)
					}
					end = start.Add(dur)
				}
			} else if d.duration != "" {
				return kit.Fail("--duration needs --start, since a length has to hang off a beginning.")
			}
			return kit.Mutate(c, ui.ResultSpec{
				Action: ui.Updated, Kind: "events", Count: 1, Name: d.title,
				IDs: []string{kit.JoinPair(calID, eventID)},
			}, func() error {
				return c.App.Calendar.EventUpdate(c.Ctx, c.U, calID, eventID,
					d.title, d.location, d.description, start, end)
			})
		}),
	}
	d.register(c, "Replace")
	return c
}

func eventsRespondCmd() *cobra.Command {
	status := &kit.Enum{
		Name: "status", Usage: "Your answer",
		Values: []string{"accept", "tentative", "decline"},
	}
	c := &cobra.Command{
		Use:   "respond REF",
		Short: "Answer an invitation, telling the organizer",
		Args:  cobra.ExactArgs(1),
		RunE: kit.Run([]kit.Step{kit.StepAuth, kit.StepExpand, kit.StepUnlock}, func(c *kit.Invocation) error {
			answer, err := status.Value()
			if err != nil {
				return err
			}
			code, err := calsvc.StatusFromFlag(answer)
			if err != nil {
				return err
			}
			calID, eventID, err := resolveEvent(c, c.Args[0])
			if err != nil {
				return err
			}
			var res *calsvc.RespondResult
			if err := kit.Mutate(c, ui.ResultSpec{
				Action: ui.Responded, Kind: "events", Count: 1,
				Detail: answer, IDs: []string{kit.JoinPair(calID, eventID)},
			}, func() error {
				var err error
				res, err = c.App.Calendar.EventRespond(c.Ctx, c.U, calID, eventID, code)
				return err
			}); err != nil {
				return err
			}
			if res != nil && res.Reply != nil {
				if err := sendICS(c, c.U, res.Reply.Recipients, res.Reply.Subject, res.Reply.Body, res.Reply.ICS, "REPLY"); err != nil {
					c.Note("You responded, but telling the organizer by email failed: %v", err)
				}
			}
			return nil
		}),
	}
	status.Register(c)
	_ = c.MarkFlagRequired("status")
	return c
}

func eventsDeleteCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "delete REF...",
		Short: "Delete events",
		Args:  cobra.MinimumNArgs(1),
		RunE: kit.Run([]kit.Step{kit.StepAuth, kit.StepExpand, kit.StepUnlock}, func(c *kit.Invocation) error {
			type target struct{ cal, event string }
			targets := make([]target, 0, len(c.Args))
			ids := make([]string, 0, len(c.Args))
			for _, ref := range c.Args {
				calID, eventID, err := resolveEvent(c, ref)
				if err != nil {
					return err
				}
				targets = append(targets, target{calID, eventID})
				ids = append(ids, kit.JoinPair(calID, eventID))
			}
			return kit.Mutate(c, ui.ResultSpec{
				Action: ui.Deleted, Kind: "events", Count: len(targets), IDs: ids,
			}, func() error {
				for _, t := range targets {
					if err := c.App.Calendar.EventDelete(c.Ctx, c.U, t.cal, t.event); err != nil {
						return err
					}
				}
				return nil
			})
		}),
	}
}

// ── helpers ──

func resolveCalendar(c *kit.Invocation, ref string) (string, error) {
	expanded, err := kit.Expand(c.App, ref)
	if err != nil {
		return "", err
	}
	return c.App.Calendar.ResolveCalendarID(c.Ctx, expanded)
}

// calendarName turns a calendar ID back into its name, so a record reports the
// calendar a person recognises rather than a second opaque ID.
func calendarName(c *kit.Invocation, id string) (string, error) {
	cals, err := c.App.Calendar.CalendarsList(c.Ctx)
	if err != nil {
		return "", err
	}
	for _, cal := range cals {
		if cal.ID == id {
			return cal.Name, nil
		}
	}
	return id, nil
}
