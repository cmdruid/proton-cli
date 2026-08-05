package cli

import (
	"github.com/roman-16/proton-cli/internal/proton"
	"github.com/roman-16/proton-cli/internal/render"
	"github.com/spf13/cobra"
)

// driveSettingsPath backs Drive's only settings page, Version history.
const driveSettingsPath = "/drive/me/settings"

var driveSettingSpecs = map[string]settingSpec{
	"version-history": {
		Path: driveSettingsPath, Field: "RevisionRetentionDays",
		Page: "Version history", Desc: "how long previous file versions are kept",
		Enum: []enumValue{{"off", 0}, {"7d", 7}, {"30d", 30}, {"180d", 180}, {"1y", 365}, {"10y", 3650}},
	},
}

func driveSettingsCmd() *cobra.Command {
	return settingsCmd("drive", "Show Drive settings", driveSettingSpecs, func(c *Invocation) error {
		resp, err := c.App.API.Do(c.Ctx, proton.Request{Method: "GET", Path: driveSettingsPath})
		if err != nil {
			return err
		}
		if c.R().Format != render.FormatText {
			return c.R().JSON(resp.Body)
		}
		return printSettingsText(c, resp.Body, renderDriveSettings)
	})
}

func renderDriveSettings(c *Invocation, m map[string]any) {
	us, _ := m["UserSettings"].(map[string]any)
	if us == nil {
		_ = c.R().Object(m)
		return
	}
	fieldPrinter(c, 16)("Version History",
		enumName(driveSettingSpecs["version-history"], us["RevisionRetentionDays"]))
}
