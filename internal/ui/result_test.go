package ui

import (
	"strings"
	"testing"
)

// The three confirmation shapes, side by side. Each is chosen by what the caller
// actually knows, and none of them ever prints "1 message(s)".
func TestResultMessageShapes(t *testing.T) {
	for _, tc := range []struct {
		name string
		spec ResultSpec
		want string
	}{{
		"named single with a kind worth saying",
		ResultSpec{Action: Created, Kind: "labels", Count: 1, Name: "Work"},
		`✓ Created label "Work".`,
	}, {
		"named single where the kind adds nothing",
		ResultSpec{Action: Uploaded, Count: 1, Name: "trail-map.txt", Detail: "to /Documents"},
		"✓ Uploaded trail-map.txt to /Documents.",
	}, {
		"a count, plural",
		ResultSpec{Action: Trashed, Kind: "messages", Count: 3, Detail: "to trash"},
		"✓ Moved 3 messages to trash.",
	}, {
		"a count of exactly one agrees in number",
		ResultSpec{Action: Trashed, Kind: "messages", Count: 1, Detail: "to trash"},
		"✓ Moved 1 message to trash.",
	}, {
		"an irregular plural",
		ResultSpec{Action: Updated, Kind: "addresses", Count: 1},
		"✓ Updated 1 address.",
	}, {
		"nothing matched",
		ResultSpec{Action: Trashed, Kind: "messages", Count: 0},
		"✓ Nothing to move.",
	}} {
		t.Run(tc.name, func(t *testing.T) {
			u, out, errb := fixture(t, Options{})
			if err := Result(u, tc.spec); err != nil {
				t.Fatal(err)
			}
			if got := strings.TrimRight(errb.String(), "\n"); got != tc.want {
				t.Errorf("got  %q\nwant %q", got, tc.want)
			}
			if out.Len() != 0 {
				t.Errorf("a confirmation belongs on stderr, got %q on stdout", out.String())
			}
		})
	}
}

// A create puts the ID on stdout and the sentence on stderr, so `ID=$(...)`
// captures the ID and nothing else.
func TestResultSplitsIDFromConfirmation(t *testing.T) {
	u, out, errb := fixture(t, Options{})
	spec := ResultSpec{
		Action: Created, Kind: "labels", Count: 1, Name: "Work",
		IDs: []string{"kQ81mDx4T9wLpN4vRs8kZc=="}, EmitID: true,
	}
	if err := Result(u, spec); err != nil {
		t.Fatal(err)
	}
	if got := out.String(); got != "kQ81mDx4T9wLpN4vRs8kZc==\n" {
		t.Errorf("stdout should be the bare ID, got %q", got)
	}
	check(t, "result_created", out, errb)
}

// An ID is data, so it is never shortened even when a terminal is attached: the
// next command may run on another machine.
func TestResultIDIsNeverShortened(t *testing.T) {
	t.Setenv("PROTON_CLI_FORCE_TTY", "1")
	u, out, _ := fixture(t, Options{})
	full := "kQ81mDx4T9wLpN4vRs8kZc=="
	if err := Result(u, ResultSpec{Action: Created, Kind: "labels", Count: 1, IDs: []string{full}, EmitID: true}); err != nil {
		t.Fatal(err)
	}
	if got := strings.TrimSpace(out.String()); got != full {
		t.Errorf("emitted ID was altered: %q", got)
	}
}

func TestResultBulk(t *testing.T) {
	u, out, errb := fixture(t, Options{})
	spec := ResultSpec{Action: Trashed, Kind: "messages", Count: 3, Detail: "to trash",
		IDs: []string{"hR8sT2vW", "kM4nP9qL", "zC7bX1yE"}}
	if err := Result(u, spec); err != nil {
		t.Fatal(err)
	}
	check(t, "result_bulk", out, errb)
}

// A dry run says what it would do and then shows exactly which things it would
// do it to, because a count alone is not enough to approve a deletion.
func TestResultDryRunShowsTheSelection(t *testing.T) {
	u, out, errb := fixture(t, Options{})
	preview := func(p *UI) error {
		return Table(p, TableSpec[message]{
			Noun: "messages", Columns: messageColumns()[:4],
			Total: Unknown, Page: Unpaged,
		}, messages())
	}
	spec := ResultSpec{
		Action: Trashed, Kind: "messages", Count: 3, Detail: "to trash",
		DryRun: true, Preview: preview,
	}
	if err := Result(u, spec); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(errb.String(), "Dry run - would move 3 messages to trash:") {
		t.Errorf("missing the dry-run line: %q", errb.String())
	}
	// A dry run answers nothing, so none of it may land on stdout: the preview
	// has to survive `--dry-run > /dev/null`.
	if out.Len() != 0 {
		t.Errorf("the preview belongs on stderr, got %q on stdout", out.String())
	}
	check(t, "result_dry_run", out, errb)
}

func TestResultDryRunWithoutPreviewEndsInAPeriod(t *testing.T) {
	u, _, errb := fixture(t, Options{})
	spec := ResultSpec{Action: Emptied, Kind: "items", Count: 12, Detail: "from the trash", DryRun: true}
	if err := Result(u, spec); err != nil {
		t.Fatal(err)
	}
	want := "Dry run - would empty 12 items from the trash.\n"
	if errb.String() != want {
		t.Errorf("got  %q\nwant %q", errb.String(), want)
	}
}

// --output json has to mean JSON even for a mutation. A bare ID here would make
// `--output json` emit something no parser accepts.
func TestResultMachineIsAlwaysStructured(t *testing.T) {
	u, out, errb := fixture(t, Options{Format: FormatJSON})
	spec := ResultSpec{
		Action: Created, Kind: "labels", Count: 1, Name: "Work",
		IDs: []string{"kQ81mDx4"}, EmitID: true,
	}
	if err := Result(u, spec); err != nil {
		t.Fatal(err)
	}
	if errb.Len() != 0 {
		t.Errorf("machine mode should write nothing to stderr, got %q", errb.String())
	}
	got := out.String()
	if !strings.HasPrefix(strings.TrimSpace(got), "{") {
		t.Errorf("machine mode must emit an object, got %q", got)
	}
	// kind is singular: it describes each id, not the collection.
	if !strings.Contains(got, `"kind": "label"`) {
		t.Errorf(`want "kind": "label" in %s`, got)
	}
	check(t, "result_created_json", out, errb)
}

// One command produces one machine document. When the answer is a record shown
// afterwards, the confirmation still reaches a reader but adds no second object
// for a parser to choke on.
func TestResultMachineIsSilentWhenTheAnswerFollows(t *testing.T) {
	spec := ResultSpec{
		Action: Linked, Kind: "links", Count: 1,
		Detail: "for /Documents", AnswerFollows: true,
	}

	u, out, errb := fixture(t, Options{Format: FormatJSON})
	if err := Result(u, spec); err != nil {
		t.Fatal(err)
	}
	if out.Len() != 0 || errb.Len() != 0 {
		t.Errorf("want no output, got stdout %q stderr %q", out.String(), errb.String())
	}

	// A dry run shows no record, so it still has to speak for itself.
	dry := spec
	dry.DryRun = true
	u, out, _ = fixture(t, Options{Format: FormatJSON})
	if err := Result(u, dry); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), `"dry_run": true`) {
		t.Errorf("a dry run must report itself, got %q", out.String())
	}

	// Text mode is unchanged: the confirmation belongs on stderr.
	u, out, errb = fixture(t, Options{})
	if err := Result(u, spec); err != nil {
		t.Fatal(err)
	}
	if out.Len() != 0 {
		t.Errorf("stdout is the record's, got %q", out.String())
	}
	if !strings.Contains(errb.String(), "Created 1 link for /Documents.") {
		t.Errorf("want the confirmation on stderr, got %q", errb.String())
	}
}

func TestResultMachineDryRunIsFlagged(t *testing.T) {
	u, out, _ := fixture(t, Options{Format: FormatJSON})
	spec := ResultSpec{Action: Trashed, Kind: "messages", Count: 2, DryRun: true,
		IDs: []string{"a", "b"}}
	if err := Result(u, spec); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), `"dry_run": true`) {
		t.Errorf("a dry run must be visible in machine output: %s", out.String())
	}
}

// Every action carries all three grammatical forms, because the confirmation,
// the preview and the JSON each need a different one.
func TestActionVocabularyIsComplete(t *testing.T) {
	seen := map[string]string{}
	for _, a := range Actions {
		if a.Past == "" || a.Verb == "" || a.Key == "" {
			t.Errorf("incomplete action: %+v", a)
		}
		if a.Past[0] < 'A' || a.Past[0] > 'Z' {
			t.Errorf("Past should open a sentence: %q", a.Past)
		}
		if strings.ToLower(a.Verb) != a.Verb {
			t.Errorf("Verb follows \"would\", so it stays lower case: %q", a.Verb)
		}
		if strings.ToLower(a.Key) != a.Key {
			t.Errorf("Key is a machine value, so it stays lower case: %q", a.Key)
		}
		if prev, dup := seen[a.Key]; dup {
			t.Errorf("duplicate action key %q (also %q)", a.Key, prev)
		}
		seen[a.Key] = a.Past
	}
}
