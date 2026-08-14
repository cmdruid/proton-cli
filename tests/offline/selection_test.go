package offline

import "testing"

// A command line that names nothing to act on, asks for something impossible, or
// contradicts itself is wrong before anybody is signed in. None of these needs an
// account to be judged, so none of them costs one.

// "Everything" is too consequential to be the default reading of an empty command
// line, and which filters a command has is part of the refusal.
func TestASelectionThatNamesNothingIsRefused(t *testing.T) {
	for _, args := range [][]string{
		{"mail", "messages", "trash"},
		{"mail", "messages", "delete"},
		{"mail", "conversations", "trash"},
		{"mail", "messages", "unschedule"},
		{"drive", "items", "delete"},
		{"pass", "items", "trash"},
		{"account", "sessions", "revoke"},
	} {
		refuses(t, 1, args, "Nothing selected")
	}
}

// A command that will decrypt still judges its own arguments first. Keys are one
// more thing to fetch rather than a state to be in, so needing them no longer
// makes "not signed in" the answer to a command line that was wrong anyway.
func TestACommandThatDecryptsStillJudgesItsArgumentsFirst(t *testing.T) {
	for _, tt := range []struct {
		args   []string
		phrase string
	}{
		{[]string{"pass", "items", "create"}, "An item needs a name"},
		{[]string{"pass", "aliases", "create"}, "An alias needs a prefix"},
		{[]string{"contacts", "create"}, "A contact needs at least a name or an email address"},
		{[]string{"calendar", "events", "create", "--calendar", "Work"}, "An event needs a title and a start"},
	} {
		refuses(t, 1, tt.args, tt.phrase)
	}
}

func TestSendingNeedsSomethingToSend(t *testing.T) {
	refuses(t, 1, []string{"mail", "messages", "send"}, "required")
	refuses(t, 1, []string{"mail", "messages", "send", "--subject", "x", "--body", "y"},
		"At least one recipient is required", "--to, --cc or --bcc")
}

// One stream cannot carry several files, and one path cannot name several. Both
// are decided by the flags alone.
func TestOneDestinationForSeveralItemsIsRefused(t *testing.T) {
	for _, args := range [][]string{
		{"mail", "messages", "attachments", "download", "--output", "-", "some-message"},
		{"mail", "messages", "attachments", "download", "--output", "/tmp/one.bin", "some-message"},
	} {
		refuses(t, 1, args, "--output-dir")
	}
}

// Two flags that mean different selections cannot both be honoured.
func TestContradictoryFlagsAreRefused(t *testing.T) {
	refuses(t, 1, []string{"account", "sessions", "revoke", "--others", "some-uid"}, "--others")
}

// The raw escape hatch judges what it was handed before it sends it.
func TestTheRawAPICommandJudgesItsOwnArguments(t *testing.T) {
	refuses(t, 1, []string{"api", "POST", "/core/v4/labels", "--body", "{not json}"},
		"not valid JSON")
	refuses(t, 1, []string{"api", "GET", "/core/v4/users", "--query", "Noequalssign"},
		"key=value")
}

// An auto-reply schedule that does not describe a schedule is refused before it is
// sent, and the refusal says which shape the chosen repeat takes.
func TestAnAutoReplyScheduleThatContradictsItselfIsRefused(t *testing.T) {
	for _, tt := range []struct {
		args    []string
		phrases []string
	}{
		{[]string{"--repeat", "permanent", "--start", "09:00", "--message", "x"}, []string{"takes no --start"}},
		{[]string{"--repeat", "daily", "--start", "09:00", "--end", "17:00", "--message", "x"}, []string{"needs --days"}},
		{[]string{"--repeat", "weekly", "--start", "mon:09:00", "--end", "fri:17:00", "--days", "mon", "--message", "x"},
			[]string{"--days applies to --repeat daily"}},
		{[]string{"--repeat", "hourly", "--message", "x"},
			[]string{"--repeat accepts: fixed, daily, weekly, monthly, permanent"}},
		{[]string{"--repeat", "permanent"}, []string{"A message is required"}},
	} {
		refuses(t, 1, append([]string{"mail", "settings", "autoreply", "set"}, tt.args...), tt.phrases...)
	}
}
