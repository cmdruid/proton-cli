package cli

import (
	"github.com/roman-16/proton-cli/internal/proton"
	"github.com/roman-16/proton-cli/internal/render"
	"github.com/spf13/cobra"
)

// calendarSettingsPath is the single endpoint behind Calendar's General page: it
// takes a partial object, so every key below writes to the same place.
const calendarSettingsPath = "/settings/calendar"

var calendarSettingSpecs = map[string]settingSpec{
	"auto-detect-timezone": {
		Path: calendarSettingsPath, Field: "AutoDetectPrimaryTimezone",
		Page: "General", Desc: "follow the system time zone", Enum: onOff(),
	},
	"auto-import-invite": {
		Path: calendarSettingsPath, Field: "AutoImportInvite",
		Page: "General", Desc: "add emailed invitations to your calendar automatically", Enum: onOff(),
	},
	"default-calendar": {
		Path: calendarSettingsPath, Field: "DefaultCalendarID",
		Page: "General", Desc: "calendar ID new events land in",
	},
	"invite-locale": {
		Path: calendarSettingsPath, Field: "InviteLocale",
		Page: "General", Desc: "language of outgoing invitations, e.g. en_US",
	},
	"primary-timezone": {
		Path: calendarSettingsPath, Field: "PrimaryTimezone",
		Page: "General", Desc: "IANA time zone the grid is drawn in, e.g. Europe/Vienna",
	},
	"secondary-timezone": {
		Path: calendarSettingsPath, Field: "SecondaryTimezone",
		Page: "General", Desc: "IANA time zone shown alongside the primary one",
	},
	"show-secondary-timezone": {
		Path: calendarSettingsPath, Field: "DisplaySecondaryTimezone",
		Page: "General", Desc: "show the secondary time zone", Enum: onOff(),
	},
	"view": {
		Path: calendarSettingsPath, Field: "ViewPreference",
		Page: "General", Desc: "default view",
		Enum: []enumValue{{"day", 0}, {"week", 1}, {"month", 2}, {"year", 3}, {"planning", 4}},
	},
	"week-numbers": {
		Path: calendarSettingsPath, Field: "DisplayWeekNumber",
		Page: "General", Desc: "show week numbers", Enum: onOff(),
	},
}

// calendarSettingsCmd is the `calendar settings` tree: the General page behind
// `set`, and the Calendars page as its own subcommand.
func calendarSettingsCmd() *cobra.Command {
	c := settingsCmd("calendar", "Show calendar settings", calendarSettingSpecs, func(c *Invocation) error {
		resp, err := c.App.API.Do(c.Ctx, proton.Request{Method: "GET", Path: calendarSettingsPath})
		if err != nil {
			return err
		}
		if c.R().Format != render.FormatText {
			return c.R().JSON(resp.Body)
		}
		return printSettingsText(c, resp.Body, renderCalendarSettings)
	})
	c.AddCommand(calendarsCmd())
	return c
}

func renderCalendarSettings(c *Invocation, m map[string]any) {
	cs, _ := m["CalendarUserSettings"].(map[string]any)
	if cs == nil {
		_ = c.R().Object(m)
		return
	}
	p := fieldPrinter(c, 24)
	p("View", enumName(calendarSettingSpecs["view"], cs["ViewPreference"]))
	p("Week Numbers", onOffText(intOf(cs["DisplayWeekNumber"])))
	p("Primary Timezone", str(cs["PrimaryTimezone"]))
	p("Auto Detect Timezone", onOffText(intOf(cs["AutoDetectPrimaryTimezone"])))
	p("Secondary Timezone", str(cs["SecondaryTimezone"]))
	p("Show Secondary", onOffText(intOf(cs["DisplaySecondaryTimezone"])))
	p("Auto Import Invites", onOffText(intOf(cs["AutoImportInvite"])))
	p("Invite Locale", str(cs["InviteLocale"]))
	p("Default Calendar", str(cs["DefaultCalendarID"]))
}
