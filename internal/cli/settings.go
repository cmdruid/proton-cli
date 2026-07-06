package cli

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"

	"github.com/roman-16/proton-cli/internal/proton"
	"github.com/roman-16/proton-cli/internal/render"
	"github.com/spf13/cobra"
)

func newSettingsCmd() *cobra.Command {
	c := &cobra.Command{Use: "settings", Short: "Account and mail settings"}
	c.AddCommand(&cobra.Command{
		Use:   "get",
		Short: "Show current account settings",
		RunE: run([]Step{stepAuth}, func(c *Invocation) error {
			resp, err := c.App.API.Do(c.Ctx, proton.Request{Method: "GET", Path: "/core/v4/settings"})
			if err != nil {
				return err
			}
			if c.R().Format != render.FormatText {
				return c.R().JSON(resp.Body)
			}
			return printSettingsText(c, resp.Body, renderAccountSettings)
		}),
	})
	c.AddCommand(&cobra.Command{
		Use:   "mail",
		Short: "Show mail settings",
		RunE: run([]Step{stepAuth}, func(c *Invocation) error {
			resp, err := c.App.API.Do(c.Ctx, proton.Request{Method: "GET", Path: "/mail/v4/settings"})
			if err != nil {
				return err
			}
			if c.R().Format != render.FormatText {
				return c.R().JSON(resp.Body)
			}
			return printSettingsText(c, resp.Body, renderMailSettings)
		}),
	})
	c.AddCommand(settingsSetCmd())
	return c
}

type settingSpec struct {
	path  string
	field string
	isInt bool
	desc  string
}

// mailSettingSpecs maps friendly keys to the per-setting Proton PUT endpoints.
var mailSettingSpecs = map[string]settingSpec{
	"page-size":            {"/mail/v4/settings/pagesize", "PageSize", true, "messages per page (50, 100, 200)"},
	"view-mode":            {"/mail/v4/settings/viewmode", "ViewMode", true, "0=conversations, 1=messages"},
	"sign":                 {"/mail/v4/settings/sign", "Sign", true, "0=off, 1=sign outgoing"},
	"attach-public-key":    {"/mail/v4/settings/attachpublic", "AttachPublicKey", true, "0/1"},
	"auto-save-contacts":   {"/mail/v4/settings/autocontacts", "AutoSaveContacts", true, "0/1"},
	"hide-remote-images":   {"/mail/v4/settings/hide-remote-images", "HideRemoteImages", true, "0/1"},
	"hide-embedded-images": {"/mail/v4/settings/hide-embedded-images", "HideEmbeddedImages", true, "0/1"},
	"draft-type":           {"/mail/v4/settings/drafttype", "MIMEType", false, "text/html or text/plain"},
	"pm-signature":         {"/mail/v4/settings/pmsignature", "PMSignature", true, "0=off, 1=on"},
	"show-moved":           {"/mail/v4/settings/moved", "ShowMoved", true, "0..3"},
	"shortcuts":            {"/mail/v4/settings/shortcuts", "Shortcuts", true, "0/1"},
	"sticky-labels":        {"/mail/v4/settings/stickylabels", "StickyLabels", true, "0/1"},
	"prompt-pin":           {"/mail/v4/settings/promptpin", "PromptPin", true, "0/1"},
	"enable-folder-color":  {"/mail/v4/settings/enablefoldercolor", "EnableFolderColor", true, "0/1"},
	"delay-send":           {"/mail/v4/settings/delaysend", "DelaySendSeconds", true, "seconds (0-20)"},
	"almost-all-mail":      {"/mail/v4/settings/almost-all-mail", "AlmostAllMail", true, "0/1"},
}

func settingsSetCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "set [KEY VALUE]",
		Short: "Update a mail setting (run with no args to list keys)",
		Args:  cobra.MaximumNArgs(2),
		RunE: run([]Step{stepAuth}, func(c *Invocation) error {
			if len(c.Args) < 2 {
				keys := make([]string, 0, len(mailSettingSpecs))
				for k := range mailSettingSpecs {
					keys = append(keys, k)
				}
				sort.Strings(keys)
				out := c.R().Stdout
				_, _ = fmt.Fprintln(out, "Available settings (settings set KEY VALUE):")
				for _, k := range keys {
					_, _ = fmt.Fprintf(out, "  %-22s %s\n", k, mailSettingSpecs[k].desc)
				}
				return nil
			}
			key, val := c.Args[0], c.Args[1]
			spec, ok := mailSettingSpecs[key]
			if !ok {
				return fmt.Errorf("unknown setting %q; run `settings set` with no args to list keys", key)
			}
			var body map[string]any
			if spec.isInt {
				n, err := strconv.Atoi(val)
				if err != nil {
					return fmt.Errorf("setting %q expects an integer value", key)
				}
				body = map[string]any{spec.field: n}
			} else {
				body = map[string]any{spec.field: val}
			}
			if c.dryRun("set %s = %s", key, val) {
				return nil
			}
			if err := c.App.API.Decode(c.Ctx, proton.Request{Method: "PUT", Path: spec.path, Body: body}, nil); err != nil {
				return err
			}
			c.R().Success(fmt.Sprintf("Set %s = %s", key, val))
			return nil
		}),
	}
}

func printSettingsText(c *Invocation, body []byte, renderer func(*Invocation, map[string]any)) error {
	var m map[string]any
	if err := json.Unmarshal(body, &m); err != nil {
		return c.R().JSON(body)
	}
	renderer(c, m)
	return nil
}

func renderAccountSettings(c *Invocation, m map[string]any) {
	u, _ := m["UserSettings"].(map[string]any)
	if u == nil {
		_ = c.R().Object(m)
		return
	}
	print := func(k, v string) { _, _ = fmt.Fprintf(c.R().Stdout, "%-16s %s\n", k+":", v) }
	print("Locale", str(u["Locale"]))
	if e, ok := u["Email"].(map[string]any); ok {
		print("Recovery Email", str(e["Value"]))
	}
	if p, ok := u["Phone"].(map[string]any); ok {
		print("Recovery Phone", str(p["Value"]))
	}
	print("Telemetry", intStr(u["Telemetry"]))
	print("CrashReports", intStr(u["CrashReports"]))
	if hs, ok := u["HighSecurity"].(map[string]any); ok {
		v, _ := hs["Value"].(float64)
		if int(v) == 1 {
			print("High Security", "on")
		} else {
			print("High Security", "off")
		}
	}
}

func renderMailSettings(c *Invocation, m map[string]any) {
	ms, _ := m["MailSettings"].(map[string]any)
	if ms == nil {
		_ = c.R().Object(m)
		return
	}
	print := func(k, v string) { _, _ = fmt.Fprintf(c.R().Stdout, "%-20s %s\n", k+":", v) }
	print("Display Name", str(ms["DisplayName"]))
	print("Page Size", intStr(ms["PageSize"]))
	print("View Mode", viewMode(intOf(ms["ViewMode"])))
	print("Draft MIME Type", str(ms["DraftMIMEType"]))
	print("PM Signature", onOff(intOf(ms["PMSignature"])))
	print("Auto Save Contacts", onOff(intOf(ms["AutoSaveContacts"])))
	print("Hide Remote Images", onOff(intOf(ms["HideRemoteImages"])))
	print("Sign Outgoing", onOff(intOf(ms["Sign"])))
	print("Attach Public Key", onOff(intOf(ms["AttachPublicKey"])))
	print("Shortcuts", onOff(intOf(ms["Shortcuts"])))
	print("Delay Send", fmt.Sprintf("%ds", intOf(ms["DelaySendSeconds"])))
}

func str(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

func intStr(v any) string {
	if f, ok := v.(float64); ok {
		return fmt.Sprintf("%d", int(f))
	}
	return ""
}

func intOf(v any) int {
	if f, ok := v.(float64); ok {
		return int(f)
	}
	return 0
}

func onOff(i int) string {
	if i == 1 {
		return "on"
	}
	return "off"
}

func viewMode(i int) string {
	if i == 0 {
		return "conversations"
	}
	return "messages"
}
