package cli

import (
	"encoding/json"
	"fmt"

	"github.com/roman-16/proton-cli/internal/proton"
	"github.com/roman-16/proton-cli/internal/render"
	mailsvc "github.com/roman-16/proton-cli/internal/service/mail"
	"github.com/spf13/cobra"
)

// mailSettingSpecs mirrors the writable scalars on Proton's Mail "General" and
// "Email privacy" settings pages. Keys the API stores through a shaped request
// rather than a plain value (image proxy, spam action) are absent; the
// structured pages get their own subcommands instead.
var mailSettingSpecs = map[string]settingSpec{
	"almost-all-mail": {
		Path: "/mail/v4/settings/almost-all-mail", Field: "AlmostAllMail",
		Page: "General", Desc: "exclude spam and trash from All mail", Enum: onOff(),
	},
	"attach-public-key": {
		Path: "/mail/v4/settings/attachpublic", Field: "AttachPublicKey",
		Page: "General", Desc: "attach your public key to outgoing mail", Enum: onOff(),
	},
	"auto-delete-spam-trash": {
		Path: "/mail/v4/settings/auto-delete-spam-and-trash-days", Field: "Days",
		Page: "General", Desc: "permanently delete spam and trash after N days",
		Enum: []enumValue{{"off", 0}, {"30d", 30}},
	},
	"auto-save-contacts": {
		Path: "/mail/v4/settings/autocontacts", Field: "AutoSaveContacts",
		Page: "General", Desc: "add unknown recipients to Contacts", Enum: onOff(),
	},
	"composer-mode": {
		Path: "/mail/v4/settings/composermode", Field: "ComposerMode",
		Page: "General", Desc: "how the web composer opens",
		Enum: []enumValue{{"popup", 0}, {"maximized", 1}},
	},
	"confirm-link": {
		Path: "/mail/v4/settings/confirmlink", Field: "ConfirmLink",
		Page: "General", Desc: "confirm before opening external links", Enum: onOff(),
	},
	"delay-send": {
		Path: "/mail/v4/settings/delaysend", Field: "DelaySendSeconds",
		Page: "General", Desc: "undo-send window", Range: &intRange{0, 20, "seconds"},
	},
	"draft-type": {
		Path: "/mail/v4/settings/drafttype", Field: "MIMEType",
		Page: "General", Desc: "default composer format", Text: []string{"text/html", "text/plain"},
	},
	"enable-folder-color": {
		Path: "/mail/v4/settings/enablefoldercolor", Field: "EnableFolderColor",
		Page: "General", Desc: "colour folders in the sidebar", Enum: onOff(),
	},
	"hide-embedded-images": {
		Path: "/mail/v4/settings/hide-embedded-images", Field: "HideEmbeddedImages",
		Page: "Email privacy", Desc: "block images embedded in messages", Enum: onOff(),
	},
	"hide-remote-images": {
		Path: "/mail/v4/settings/hide-remote-images", Field: "HideRemoteImages",
		Page: "Email privacy", Desc: "block images loaded from the internet", Enum: onOff(),
	},
	"hide-sender-images": {
		Path: "/mail/v4/settings/hide-sender-images", Field: "HideSenderImages",
		Page: "Email privacy", Desc: "block sender profile pictures", Enum: onOff(),
	},
	"inherit-folder-color": {
		Path: "/mail/v4/settings/inheritparentfoldercolor", Field: "InheritParentFolderColor",
		Page: "General", Desc: "subfolders inherit their parent's colour", Enum: onOff(),
	},
	"message-buttons": {
		Path: "/mail/v4/settings/messagebuttons", Field: "MessageButtons",
		Page: "General", Desc: "order of the read/unread buttons",
		Enum: []enumValue{{"read-unread", 0}, {"unread-read", 1}},
	},
	"page-size": {
		Path: "/mail/v4/settings/pagesize", Field: "PageSize",
		Page: "General", Desc: "messages per page",
		Enum: []enumValue{{"50", 50}, {"100", 100}, {"200", 200}},
	},
	"pm-signature": {
		Path: "/mail/v4/settings/pmsignature", Field: "PMSignature",
		Page: "General", Desc: `append "Sent with Proton Mail secure email."`, Enum: onOff(),
	},
	"prompt-pin": {
		Path: "/mail/v4/settings/promptpin", Field: "PromptPin",
		Page: "General", Desc: "offer to pin keys of contacts who sign their mail", Enum: onOff(),
	},
	"shortcuts": {
		Path: "/mail/v4/settings/shortcuts", Field: "Shortcuts",
		Page: "General", Desc: "keyboard shortcuts in the web client", Enum: onOff(),
	},
	"show-moved": {
		Path: "/mail/v4/settings/moved", Field: "ShowMoved",
		Page: "General", Desc: "keep moved drafts and sent mail in their folders",
		Enum: []enumValue{{"none", 0}, {"drafts", 1}, {"sent", 2}, {"drafts-and-sent", 3}},
	},
	"sign": {
		Path: "/mail/v4/settings/sign", Field: "Sign",
		Page: "General", Desc: "sign outgoing mail by default", Enum: onOff(),
	},
	"sticky-labels": {
		Path: "/mail/v4/settings/stickylabels", Field: "StickyLabels",
		Page: "General", Desc: "keep a label when moving a message", Enum: onOff(),
	},
	"unread-favicon": {
		Path: "/mail/v4/settings/unread-favicon", Field: "UnreadFavicon",
		Page: "General", Desc: "show the unread count in the browser tab", Enum: onOff(),
	},
	"view-layout": {
		Path: "/mail/v4/settings/viewlayout", Field: "ViewLayout",
		Page: "General", Desc: "mailbox layout",
		Enum: []enumValue{{"column", 0}, {"row", 1}},
	},
	"view-mode": {
		Path: "/mail/v4/settings/viewmode", Field: "ViewMode",
		Page: "General", Desc: "group mail into threads or list single messages",
		Enum: []enumValue{{"conversations", 0}, {"messages", 1}},
	},
}

// mailSettingsCmd is the `mail settings` tree: one subcommand per page on
// Proton's Mail settings, with the scalar pages behind `set`.
func mailSettingsCmd() *cobra.Command {
	c := settingsCmd("mail", "Show mail settings", mailSettingSpecs, func(c *Invocation) error {
		resp, err := c.App.API.Do(c.Ctx, proton.Request{Method: "GET", Path: "/mail/v4/settings"})
		if err != nil {
			return err
		}
		if c.R().Format != render.FormatText {
			return c.R().JSON(resp.Body)
		}
		return printSettingsText(c, resp.Body, renderMailSettings)
	})
	c.AddCommand(mailAddressesCmd(), mailLabelsCmd(), mailFiltersCmd(), mailAutoreplyCmd())
	return c
}

func renderMailSettings(c *Invocation, m map[string]any) {
	ms, _ := m["MailSettings"].(map[string]any)
	if ms == nil {
		_ = c.R().Object(m)
		return
	}
	p := fieldPrinter(c, 22)
	p("Display Name", str(ms["DisplayName"]))
	p("Page Size", enumName(mailSettingSpecs["page-size"], ms["PageSize"]))
	p("View Mode", enumName(mailSettingSpecs["view-mode"], ms["ViewMode"]))
	p("View Layout", enumName(mailSettingSpecs["view-layout"], ms["ViewLayout"]))
	p("Draft Format", str(ms["DraftMIMEType"]))
	p("Show Moved", enumName(mailSettingSpecs["show-moved"], ms["ShowMoved"]))
	p("Proton Footer", onOffText(intOf(ms["PMSignature"])&1))
	p("Sign Outgoing", onOffText(intOf(ms["Sign"])))
	p("Attach Public Key", onOffText(intOf(ms["AttachPublicKey"])))
	p("Auto Save Contacts", onOffText(intOf(ms["AutoSaveContacts"])))
	p("Hide Remote Images", onOffText(intOf(ms["HideRemoteImages"])))
	p("Hide Embedded Images", onOffText(intOf(ms["HideEmbeddedImages"])))
	p("Shortcuts", onOffText(intOf(ms["Shortcuts"])))
	p("Delay Send", fmt.Sprintf("%ds", intOf(ms["DelaySendSeconds"])))
	p("Auto-reply", autoReplySummary(ms["AutoResponder"]))
}

// autoReplySummary renders the one-line auto-reply status shown by
// `mail settings`, pointing at the subcommand that manages it.
func autoReplySummary(v any) string {
	raw, ok := v.(map[string]any)
	if !ok {
		return "off"
	}
	b, err := json.Marshal(raw)
	if err != nil {
		return "off"
	}
	ar, err := mailsvc.DecodeAutoReply(b)
	if err != nil || !ar.Enabled {
		return "off"
	}
	return "on (" + ar.ScheduleSummary() + ")"
}
