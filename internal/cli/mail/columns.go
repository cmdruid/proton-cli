package mail

import (
	"strconv"
	"strings"

	mailsvc "github.com/roman-16/proton-cli/internal/service/mail"
	"github.com/roman-16/proton-cli/internal/ui"
	"github.com/roman-16/proton-cli/internal/units"
)

// The mail tables.
//
// Three rules hold across all of them, and across every other collection in the
// CLI: the ID is the first column and is called ID; dates go through one
// formatter; and status is a FLAGS column of glyphs rather than an unpronounceable
// header.

// flags renders the compact status markers. Being last matters: a couple of these
// glyphs occupy two terminal cells while counting as one rune, so anything after
// them could sit a column off.
func flags(unread, starred bool, attachments int) string {
	var b strings.Builder
	if unread {
		b.WriteString(ui.GlyphUnread)
	}
	if starred {
		b.WriteString(ui.GlyphStarred)
	}
	if attachments > 0 {
		b.WriteString(ui.GlyphAttachment)
	}
	return b.String()
}

func messageColumns() []ui.Column[mailsvc.Message] {
	return []ui.Column[mailsvc.Message]{
		{Header: "ID", ID: true, Cell: func(m mailsvc.Message) string { return m.ID }},
		{Header: "FROM", Flex: true, Cell: func(m mailsvc.Message) string {
			if m.FromName != "" {
				return m.FromName
			}
			return m.FromAddress
		}},
		{Header: "SUBJECT", Flex: true, Cell: func(m mailsvc.Message) string { return m.Subject }},
		{Header: "DATE", Cell: func(m mailsvc.Message) string { return units.Time(m.Time) }},
		{Header: "FLAGS", Accent: true, Cell: func(m mailsvc.Message) string {
			return flags(m.Unread == 1, m.Starred(), m.NumAttachments)
		}},
	}
}

func conversationColumns() []ui.Column[mailsvc.Conversation] {
	return []ui.Column[mailsvc.Conversation]{
		{Header: "ID", ID: true, Cell: func(c mailsvc.Conversation) string { return c.ID }},
		{Header: "FROM", Flex: true, Cell: func(c mailsvc.Conversation) string { return firstSender(c) }},
		{Header: "SUBJECT", Flex: true, Cell: func(c mailsvc.Conversation) string { return c.Subject }},
		{Header: "MESSAGES", Right: true, Cell: func(c mailsvc.Conversation) string {
			return strconv.Itoa(c.NumMessages)
		}},
		{Header: "DATE", Cell: func(c mailsvc.Conversation) string { return units.Time(c.Time) }},
		{Header: "FLAGS", Accent: true, Cell: func(c mailsvc.Conversation) string {
			return flags(c.NumUnread > 0, c.Starred(), c.NumAttachments)
		}},
	}
}

// draftColumns show recipients rather than senders: every draft is from you, so a
// FROM column would repeat one address down the page.
func draftColumns() []ui.Column[mailsvc.Message] {
	return []ui.Column[mailsvc.Message]{
		{Header: "ID", ID: true, Cell: func(m mailsvc.Message) string { return m.ID }},
		{Header: "SUBJECT", Flex: true, Cell: func(m mailsvc.Message) string { return m.Subject }},
		{Header: "SAVED", Cell: func(m mailsvc.Message) string { return units.Time(m.Time) }},
		{Header: "FLAGS", Accent: true, Cell: func(m mailsvc.Message) string {
			return flags(false, m.Starred(), m.NumAttachments)
		}},
	}
}

func firstSender(c mailsvc.Conversation) string {
	if len(c.Senders) == 0 {
		return ""
	}
	if n, ok := c.Senders[0]["Name"].(string); ok && n != "" {
		return n
	}
	if a, ok := c.Senders[0]["Address"].(string); ok {
		return a
	}
	return ""
}
