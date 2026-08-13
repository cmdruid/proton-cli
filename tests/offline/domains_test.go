package offline

import "testing"

// A value the interface declares a domain for is judged against that domain
// before anything is sent, and the refusal names the whole domain: somebody who
// guessed wrong needs the list, not the news that they were wrong.

func TestSettingValueOutsideItsDomainIsRefused(t *testing.T) {
	for _, tt := range []struct {
		args    []string
		phrases []string
	}{
		{[]string{"mail", "settings", "set", "view-mode", "threads"}, []string{"conversations", "messages"}},
		{[]string{"mail", "settings", "set", "view-mode", "7"}, []string{"conversations", "messages"}},
		{[]string{"mail", "settings", "set", "delay-send", "999"}, []string{"0-20 (seconds)"}},
		{[]string{"mail", "settings", "set", "page-size", "3"}, []string{"50", "100", "200"}},
		{[]string{"mail", "settings", "set", "draft-type", "text/markdown"}, []string{"text/html", "text/plain"}},
		{[]string{"account", "settings", "set", "week-start", "funday"},
			[]string{"week-start accepts", "monday", "sunday"}},
	} {
		refuses(t, 1, tt.args, tt.phrases...)
	}
}

// A key that does not exist is a reference that matched nothing, so it exits 3
// like every other miss and points at the list of the ones that do.
func TestUnknownSettingKeyIsRefusedAndPointsAtTheList(t *testing.T) {
	refuses(t, 3, []string{"mail", "settings", "set", "no-such-key", "1"},
		"no mail setting called", "mail settings list")
	refuses(t, 3, []string{"account", "settings", "set", "no-such-key", "on"},
		"no account setting called", "settings list")
	refuses(t, 3, []string{"calendar", "settings", "set", "no-such-key", "on"},
		"no calendar setting called", "calendar settings list")
	refuses(t, 3, []string{"drive", "settings", "set", "no-such-key", "on"},
		"no drive setting called", "drive settings list")
}

func TestSettingSomethingNeedsAKeyAndAValue(t *testing.T) {
	refuses(t, 1, []string{"account", "settings", "set"},
		"KEY and a VALUE", "account settings list")
}

func TestFlagValueOutsideItsDomainIsRefused(t *testing.T) {
	for _, tt := range []struct {
		args    []string
		phrases []string
	}{
		{[]string{"mail", "messages", "get", "--format", "wut", "any-ref"},
			[]string{"--format accepts:", "text", "html", "raw"}},
		{[]string{"calendar", "events", "respond", "--status", "maybe", "any-ref"},
			[]string{"--status accepts:", "accept", "tentative", "decline"}},
		{[]string{"drive", "photos", "list", "--tag", "bogus"},
			[]string{"--tag accepts:", "favorites"}},
		{[]string{"--output", "xml", "mail", "messages", "list"},
			[]string{"--output accepts:", "text", "json", "yaml"}},
	} {
		refuses(t, 1, tt.args, tt.phrases...)
	}
}

// A tag is referenced by name only, so Proton's own number for it is refused
// too: the CLI neither accepts nor emits a raw enum.
func TestPhotoTagIsRefusedAsANumber(t *testing.T) {
	refuses(t, 1, []string{"drive", "photos", "list", "--tag", "2"}, "--tag accepts:")
}

// Proton allows only its own accent colours for a label, folder, calendar or
// contact group, so anything else is refused here rather than by the server.
func TestColourOffProtonsPaletteIsRefused(t *testing.T) {
	for _, args := range [][]string{
		{"mail", "settings", "labels", "create", "--name", "x", "--color", "#FFF000"},
		{"mail", "settings", "folders", "create", "--name", "x", "--color", "not-a-colour"},
		{"calendar", "settings", "calendars", "create", "--name", "x", "--color", "#123456"},
	} {
		refuses(t, 1, args, "not a Proton accent color")
	}
}

func TestWrongNumberOfArgumentsIsRefused(t *testing.T) {
	refuses(t, 1, []string{"api"}, "arg")
	refuses(t, 1, []string{"mail", "messages", "get"}, "arg")
}
