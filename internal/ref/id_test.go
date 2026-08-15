package ref

import (
	"strings"
	"testing"
)

// The shapes below are real. Every ID in this file was taken from a live Proton
// account rather than invented, so the predicates are tested against what Proton
// issues instead of against what the code already believes.
const (
	// The common form: 64 bytes of base64url, padded. Messages, conversations,
	// labels, folders, addresses, contacts, calendars, events, Pass shares and
	// items, vaults.
	realMessage = "fYNK4pNCe4wTZwZJT9wLpN4vRs8kZc1yXd7fGh3jAe6bUi0oQm2nWr5tYvQx8mKd4vBn6cXs1wYf5hJ3gAe7bQ=="
	realVault   = "-x76EpiVSJf2oHzHgyC2D_jF8Oi0yWMKsQdUvh1axN5Xx2bDFGUd-4ArpN5CZZrPXRvP6aMQjV8cgTDEvXBRQw=="
	realItem    = "_fb26gvMWjnM7US4_wpTNm_LqIAx5LJk9c0Rj7dUR4eobSmnTl1qNsiczzmIpgio08-67uw8sneSwGLPwkN5Vw=="
	// Drive links and share invitations: 16 bytes, unpadded.
	realDriveLink = "-Qt-s7R_oGCru5u3Kv6Y8Q"
	// A session UID: 32 lowercase alphanumerics, so it can never lead with a
	// dash and never needs protecting from the flag parser.
	realSession = "f67sujpymjfyxxpa4k2wqzvhn7cd8rgb"
)

func TestFullRecognisesEveryShapeProtonIssues(t *testing.T) {
	for name, id := range map[string]string{
		"a message":         realMessage,
		"a Pass share":      realVault,
		"a Pass item":       realItem,
		"a Drive link":      realDriveLink,
		"a session UID":     realSession,
		"a padded 88-char":  strings.Repeat("a", 86) + "==",
		"an unpadded 22":    strings.Repeat("a", 22),
		"a 32-char session": strings.Repeat("a", 32),
	} {
		if !Full(id) {
			t.Errorf("%s should be a complete ID: %q", name, id)
		}
	}
}

func TestFullRejectsWhatIsNotAnID(t *testing.T) {
	for name, s := range map[string]string{
		"empty":                  "",
		"a name":                 "Personal",
		"a subject":              "Invoice #2291",
		"a short prefix":         "fYNK4pNC",
		"a path":                 "/Documents/report.pdf",
		"a compound reference":   realVault + "/" + realItem,
		"a non-base64 character": strings.Repeat("a", 20) + "!" + "=",
		"the wrong length":       strings.Repeat("a", 21),
	} {
		if Full(s) {
			t.Errorf("%s should not be a complete ID: %q", name, s)
		}
	}
}

// The two predicates partition what they accept: a token cannot be both a whole
// ID and the beginning of one, or Expand would look up something it already has.
func TestFullAndShortAreMutuallyExclusive(t *testing.T) {
	for _, s := range []string{
		realMessage, realVault, realDriveLink, realSession,
		"fYNK4pNC", "Personal", "", "hi", strings.Repeat("a", 30),
	} {
		if Full(s) && Short(s) {
			t.Errorf("both predicates matched %q", s)
		}
	}
}

// Short is deliberately loose: a search term that happens to look like a prefix
// costs nothing, because a lookup is only used when it hits.
func TestShort(t *testing.T) {
	for _, tc := range []struct {
		name string
		in   string
		want bool
	}{
		{"a real prefix", "fYNK4pNC", true},
		{"a longer prefix", "abc12345_xy7zSTUFF-base64body", true},
		{"a name that looks like one", "Personal", true},
		{"a hyphenated name", "john-doe-2026", true},
		{"seven characters", "AbC1234", false},
		{"a complete ID", realMessage, false},
		{"a Drive link", realDriveLink, false},
		{"a session UID", realSession, false},
		{"a space", "abc 1234", false},
		{"a slash", "abc/1234", false},
		{"a plus", "abc+1234", false},
		{"empty", "", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := Short(tc.in); got != tc.want {
				t.Errorf("Short(%q) = %v, want %v", tc.in, got, tc.want)
			}
		})
	}
}

func TestSplitAndJoinRoundTrip(t *testing.T) {
	for _, tc := range []struct {
		name       string
		in         string
		parts      []string
		occurrence string
	}{
		{"a bare ID", realMessage, []string{realMessage}, ""},
		{"a compound reference", realVault + "/" + realItem, []string{realVault, realItem}, ""},
		{"a human handle", "Invoice #2291", []string{"Invoice #2291"}, ""},
		{
			"an event occurrence",
			"yHkr7jKP/9F9yIxI0@2026-04-22T09:00",
			[]string{"yHkr7jKP", "9F9yIxI0"},
			"2026-04-22T09:00",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			parts, occurrence := Split(tc.in)
			if strings.Join(parts, "|") != strings.Join(tc.parts, "|") {
				t.Errorf("Split(%q) parts = %q, want %q", tc.in, parts, tc.parts)
			}
			if occurrence != tc.occurrence {
				t.Errorf("Split(%q) occurrence = %q, want %q", tc.in, occurrence, tc.occurrence)
			}
			if got := Join(parts...); got != strings.Join(tc.parts, Compound) {
				t.Errorf("Join round trip = %q", got)
			}
		})
	}
}

// Unambiguous is what decides whether the cli layer rewrites argv, so it has to
// be exactly right in both directions: a missed reference is a command that
// fails on a valid ID, and a false positive is a flag treated as data.
func TestUnambiguousAcceptsWhatOnlyAReferenceCanBe(t *testing.T) {
	for name, s := range map[string]string{
		"a dash-leading Pass share":     "-" + strings.TrimPrefix(realVault, "-"),
		"a dash-leading Drive link":     realDriveLink,
		"a dash-leading compound short": "-x76EpiV/_fb26gvM",
		"a dash-leading compound full":  realVault + "/" + realItem,
		"an occurrence":                 "-x76EpiV/_fb26gvM@2026-04-22T09:00",
	} {
		if !Unambiguous(s) {
			t.Errorf("%s should be unmistakable: %q", name, s)
		}
	}
}

func TestUnambiguousRejectsAnythingAFlagCouldBe(t *testing.T) {
	for name, s := range map[string]string{
		"a long flag":         "--album",
		"the end of flags":    "--",
		"a shorthand":         "-o",
		"a shorthand cluster": "-abc",
		"a short ID":          "-x76EpiV",
		"a bare dash":         "-",
		"empty":               "",
		"not dash-leading":    realMessage,
		"a negative number":   "-30",
		// Shapes that are close to an ID without being one. Each was a real
		// case before the grammar was written down in one place.
		"a padded ID one character short":           "-bJxDLEMvt-Z6t4Yna7V8SYQ_FIHWT2_QbBr-whe-bIE8rbZunzr5RhXGaihvQ43z2qcxcqFgVRwi7A",
		"a long token with illegal chars":           "-bJxDLEMvt-Z6t4Yna7V8SYQ_FIHWT2_QbBr!whe$bIE8rbZunzr5RhXGaihvQ43z2qcxcqFgVRwi7A==",
		"a word of a Drive link's neighbour":        "-nearly-an-invitation",
		"twenty-two characters that are not base64": "-not.an.invitation.id!",
		"a flag carrying a value that ends in ==":   "--name=A Long Title Goes Here aaaaaaaa==",
	} {
		if Unambiguous(s) {
			t.Errorf("%s must not be treated as a reference: %q", name, s)
		}
	}
}
