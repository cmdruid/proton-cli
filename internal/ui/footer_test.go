package ui

import "testing"

func TestFooter(t *testing.T) {
	for _, tc := range []struct {
		name string
		spec FooterSpec
		want string
	}{
		{"nothing found", FooterSpec{Noun: "messages", Count: 0, Total: 0, Page: 0, PageSize: 25},
			"No messages."},
		{"one page", FooterSpec{Noun: "messages", Count: 12, Total: 12, Page: 0, PageSize: 25},
			"12 messages."},
		{"one result", FooterSpec{Noun: "messages", Count: 1, Total: 1, Page: 0, PageSize: 25},
			"1 message."},
		{"more pages", FooterSpec{Noun: "messages", Count: 25, Total: 312, Page: 0, PageSize: 25},
			"25 of 312 messages. Next page: --page 1"},
		{"last page", FooterSpec{Noun: "messages", Count: 12, Total: 312, Page: 12, PageSize: 25},
			"12 of 312 messages. (last page)"},
		{"unpaginated", FooterSpec{Noun: "contacts", Count: 7, Total: Unknown, Page: Unpaged},
			"7 contacts."},
		{"search under the cap", FooterSpec{Noun: "messages", Count: 12, Total: Unknown, Page: Unpaged, Limit: 25},
			"12 messages."},
		{"search at the cap", FooterSpec{Noun: "messages", Count: 25, Total: Unknown, Page: Unpaged, Limit: 25},
			"25 messages. More may exist; raise --limit."},
		{"irregular plural", FooterSpec{Noun: "addresses", Count: 1, Total: Unknown, Page: Unpaged},
			"1 address."},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := Footer(tc.spec); got != tc.want {
				t.Errorf("got  %q\nwant %q", got, tc.want)
			}
		})
	}
}

// Every collection noun in the CLI has to survive the singulariser, since it
// feeds both the confirmation wording and the JSON "kind".
func TestSingularCoversEveryCollectionNoun(t *testing.T) {
	for plural, want := range map[string]string{
		"messages":      "message",
		"conversations": "conversation",
		"drafts":        "draft",
		"attachments":   "attachment",
		"addresses":     "address",
		"folders":       "folder",
		"labels":        "label",
		"filters":       "filter",
		"items":         "item",
		"folders/":      "folders/",
		"revisions":     "revision",
		"invitations":   "invitation",
		"photos":        "photo",
		"albums":        "album",
		"events":        "event",
		"calendars":     "calendar",
		"contacts":      "contact",
		"groups":        "group",
		"keys":          "key",
		"vaults":        "vault",
		"aliases":       "alias",
		"sessions":      "session",
		"profiles":      "profile",
		"settings":      "setting",
	} {
		if got := Singular(plural); got != want {
			t.Errorf("Singular(%q) = %q, want %q", plural, got, want)
		}
	}
}

func TestQuantityAgreesInNumber(t *testing.T) {
	for _, tc := range []struct {
		n      int
		plural string
		want   string
	}{
		{0, "messages", "0 messages"},
		{1, "messages", "1 message"},
		{2, "messages", "2 messages"},
		{1, "addresses", "1 address"},
		{3, "addresses", "3 addresses"},
	} {
		if got := Quantity(tc.n, tc.plural); got != tc.want {
			t.Errorf("Quantity(%d, %q) = %q, want %q", tc.n, tc.plural, got, tc.want)
		}
	}
}
