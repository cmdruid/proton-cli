package calendar

import (
	"context"
	"encoding/json"
	"strconv"

	"github.com/roman-16/proton-cli/internal/app"
	"github.com/roman-16/proton-cli/internal/cli/kit"
	"github.com/roman-16/proton-cli/internal/proton"
	calsvc "github.com/roman-16/proton-cli/internal/service/calendar"
	"github.com/roman-16/proton-cli/internal/ui"
	"github.com/spf13/cobra"
)

const settingsPath = "/settings/calendar"

// Calendar's General page takes a partial object, so every key writes to the same
// endpoint.
var specs = map[string]kit.Setting{
	"auto-detect-timezone": {
		Path: settingsPath, Field: "AutoDetectPrimaryTimezone",
		Page: "General", Desc: "Follow the system time zone", Enum: kit.OnOffChoices(),
	},
	"auto-import-invite": {
		Path: settingsPath, Field: "AutoImportInvite",
		Page: "General", Desc: "Add emailed invitations to your calendar automatically",
		Enum: kit.OnOffChoices(),
	},
	"default-calendar": {
		Path: settingsPath, Field: "DefaultCalendarID",
		Page: "General", Desc: "Which calendar new events land in, by ID",
	},
	"invite-locale": {
		Path: settingsPath, Field: "InviteLocale",
		Page: "General", Desc: "Language of outgoing invitations, e.g. en_US",
	},
	"primary-timezone": {
		Path: settingsPath, Field: "PrimaryTimezone",
		Page: "General", Desc: "IANA time zone the grid is drawn in, e.g. Europe/Vienna",
	},
	"secondary-timezone": {
		Path: settingsPath, Field: "SecondaryTimezone",
		Page: "General", Desc: "IANA time zone shown alongside the primary one",
	},
	"show-secondary-timezone": {
		Path: settingsPath, Field: "DisplaySecondaryTimezone",
		Page: "General", Desc: "Show the secondary time zone", Enum: kit.OnOffChoices(),
	},
	"view": {
		Path: settingsPath, Field: "ViewPreference",
		Page: "General", Desc: "Which view the web client opens on",
		Enum: kit.Ordered("day", "week", "month", "year", "planning"),
	},
	"week-numbers": {
		Path: settingsPath, Field: "DisplayWeekNumber",
		Page: "General", Desc: "Show week numbers", Enum: kit.OnOffChoices(),
	},
}

type settingsView struct {
	View               string `json:"view"`
	WeekNumbers        string `json:"week_numbers"`
	PrimaryTimezone    string `json:"primary_timezone"`
	AutoDetectTimezone string `json:"auto_detect_timezone"`
	SecondaryTimezone  string `json:"secondary_timezone,omitempty"`
	ShowSecondary      string `json:"show_secondary_timezone"`
	AutoImportInvite   string `json:"auto_import_invite"`
	InviteLocale       string `json:"invite_locale,omitempty"`
	DefaultCalendar    string `json:"default_calendar,omitempty"`
}

func settingsCmd() *cobra.Command {
	c := kit.Settings("calendar", "How Calendar behaves", specs, func(c *kit.Invocation) error {
		resp, err := c.App.API.Do(c.Ctx, proton.Request{Method: "GET", Path: settingsPath})
		if err != nil {
			return err
		}
		var env struct {
			CalendarUserSettings struct {
				ViewPreference            any
				DisplayWeekNumber         any
				PrimaryTimezone           string
				AutoDetectPrimaryTimezone any
				SecondaryTimezone         string
				DisplaySecondaryTimezone  any
				AutoImportInvite          any
				InviteLocale              string
				DefaultCalendarID         string
			}
		}
		if err := json.Unmarshal(resp.Body, &env); err != nil {
			return err
		}
		s := env.CalendarUserSettings
		view := settingsView{
			View:               specs["view"].Name(s.ViewPreference),
			WeekNumbers:        kit.OnOffText(kit.IntOf(s.DisplayWeekNumber)),
			PrimaryTimezone:    s.PrimaryTimezone,
			AutoDetectTimezone: kit.OnOffText(kit.IntOf(s.AutoDetectPrimaryTimezone)),
			SecondaryTimezone:  s.SecondaryTimezone,
			ShowSecondary:      kit.OnOffText(kit.IntOf(s.DisplaySecondaryTimezone)),
			AutoImportInvite:   kit.OnOffText(kit.IntOf(s.AutoImportInvite)),
			InviteLocale:       s.InviteLocale,
			DefaultCalendar:    s.DefaultCalendarID,
		}
		return kit.Show(c, ui.RecordSpec{
			Object: view,
			Fields: []ui.Field{
				{Label: "View", Value: view.View},
				{Label: "Week Numbers", Value: view.WeekNumbers, Always: true},
				{Label: "Primary Time Zone", Value: view.PrimaryTimezone},
				{Label: "Auto-detect Time Zone", Value: view.AutoDetectTimezone, Always: true},
				{Label: "Secondary Time Zone", Value: view.SecondaryTimezone},
				{Label: "Show Secondary", Value: view.ShowSecondary, Always: true},
				{Label: "Auto-import Invitations", Value: view.AutoImportInvite, Always: true},
				{Label: "Invitation Language", Value: view.InviteLocale},
				{Label: "Default Calendar", Value: view.DefaultCalendar, ID: true},
			},
		})
	})
	c.AddCommand(calendarsCmd())
	return c
}

// ── calendar settings calendars ──

func calendarsCmd() *cobra.Command {
	c := &cobra.Command{Use: "calendars", Short: "The calendars you keep events in"}
	c.AddCommand(calendarsListCmd(), calendarsCreateCmd(), calendarsUpdateCmd(), calendarsDeleteCmd())
	return c
}

func calendarColumns() []ui.Column[calsvc.Calendar] {
	return []ui.Column[calsvc.Calendar]{
		{Header: "ID", ID: true, Cell: func(cal calsvc.Calendar) string { return cal.ID }},
		{Header: "NAME", Flex: true, Cell: func(cal calsvc.Calendar) string { return cal.Name }},
		{Header: "COLOR", Cell: func(cal calsvc.Calendar) string { return cal.Color }},
		{Header: "MEMBERS", Right: true, Cell: func(cal calsvc.Calendar) string {
			return strconv.Itoa(cal.MemberCount)
		}},
	}
}

func calendarList(c *kit.Invocation) *kit.Lookup[calsvc.Calendar] {
	return &kit.Lookup[calsvc.Calendar]{
		Kind:   "calendar",
		Load:   func(ctx context.Context) ([]calsvc.Calendar, error) { return c.App.Calendar.CalendarsList(ctx) },
		ID:     func(cal calsvc.Calendar) string { return cal.ID },
		Handle: func(cal calsvc.Calendar) string { return cal.Name },
	}
}

func calendarsListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List your calendars",
		Args:  cobra.NoArgs,
		RunE: kit.Run(nil, func(c *kit.Invocation) error {
			cals, err := calendarList(c).Rows(c.Ctx)
			if err != nil {
				return err
			}
			return kit.List(c, ui.TableSpec[calsvc.Calendar]{
				Noun: "calendars", Columns: calendarColumns(),
				Total: ui.Unknown, Page: ui.Unpaged,
			}, cals, func(cal calsvc.Calendar) []string { return []string{cal.ID} })
		}),
	}
}

func calendarsCreateCmd() *cobra.Command {
	var name string
	color := &kit.Color{Name: "color", Default: kit.DefaultAccentColor}
	c := &cobra.Command{
		Use:   "create",
		Short: "Create a calendar",
		Args:  cobra.NoArgs,
		RunE: kit.Run(nil, func(c *kit.Invocation) error {
			if name == "" {
				return kit.Fail("A calendar needs a name.").Hint("--name Work")
			}
			return kit.Create(c, ui.ResultSpec{
				Action: ui.Created, Kind: "calendars", Name: name,
			}, func() (string, error) {
				return c.App.Calendar.CalendarCreate(c.Ctx, name, color.Value())
			})
		}),
	}
	c.Flags().StringVar(&name, "name", "", "Name for the new calendar")
	color.Register(c)
	return c
}

func calendarsUpdateCmd() *cobra.Command {
	var name string
	color := &kit.Color{Name: "color", Usage: "New accent color, as a hex value"}
	c := &cobra.Command{
		Use:   "update REF",
		Short: "Rename or recolor a calendar",
		Args:  cobra.ExactArgs(1),
		RunE: kit.Run([]kit.Step{kit.StepExpand}, func(c *kit.Invocation) error {
			if name == "" && !color.Set() {
				return kit.Fail("Nothing to change.").Hint("pass --name or --color.")
			}
			return kit.Mutate(c, ui.ResultSpec{
				Action: ui.Updated, Kind: "calendars", Count: 1, Name: name,
				IDs: []string{c.Args[0]},
			}, func() error {
				return c.App.Calendar.CalendarRename(c.Ctx, c.Args[0], name, color.Value())
			})
		}),
	}
	c.Flags().StringVar(&name, "name", "", "New name")
	color.Register(c)
	return c
}

func calendarsDeleteCmd() *cobra.Command {
	var reauth kit.Reauth
	c := &cobra.Command{
		Use:   "delete REF...",
		Short: "Delete calendars, and every event in them",
		Long: "Delete calendars, and every event in them.\n\n" +
			"Proton guards this behind an elevated session, so it asks for your password\n" +
			"even when a saved session already exists. With no terminal to ask, pass\n" +
			"--password-file or --password-stdin.",
		Args: cobra.MinimumNArgs(1),
		RunE: kit.Run([]kit.Step{kit.StepExpand}, func(c *kit.Invocation) error {
			if err := reauth.Supply(c); err != nil {
				return err
			}
			sel, err := kit.SelectFrom(c, "calendars", calendarColumns(), calendarList(c))
			if err != nil {
				return err
			}
			return kit.Mutate(c, ui.ResultSpec{
				Action: ui.Deleted, Kind: "calendars", Count: sel.Len(), IDs: sel.IDs,
				Name:    kit.Sole(sel.Rows, func(cal calsvc.Calendar) string { return cal.Name }),
				Preview: sel.Preview(),
			}, func() error {
				// Nothing here arranges the elevation: the client does it when the
				// server asks, and drops the scope again afterwards. All this owes
				// the user is a reason for the prompt.
				ctx := app.WithScopeReason(c.Ctx, "delete a calendar")
				for _, id := range sel.IDs {
					if err := c.App.Calendar.CalendarDelete(ctx, id); err != nil {
						return err
					}
				}
				return nil
			})
		}),
	}
	reauth.Declare(c)
	return c
}
