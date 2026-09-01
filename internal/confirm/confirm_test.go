package confirm

import "testing"

var (
	deleteMessages  = Command{Path: []string{"mail", "messages", "delete"}, Mutating: true, Irreversible: true}
	sendDraft       = Command{Path: []string{"mail", "drafts", "send"}, Mutating: true}
	listMessages    = Command{Path: []string{"mail", "messages", "list"}}
	deleteDriveItem = Command{Path: []string{"drive", "items", "delete"}, Mutating: true, Irreversible: true}
	listDriveItems  = Command{Path: []string{"drive", "items", "list"}}
)

func policy(t *testing.T, sources ...string) Policy {
	t.Helper()
	var p Policy
	for _, s := range sources {
		source, err := Parse(s)
		if err != nil {
			t.Fatalf("Parse(%q): %v", s, err)
		}
		p = append(p, source)
	}
	return p
}

// The classes are cut so that any command falls in some of them and not others,
// which is what makes a policy sayable at all.
func TestClassesCoverWhatTheyName(t *testing.T) {
	for _, tc := range []struct {
		policy string
		cmd    Command
		want   Outcome
	}{
		{"mutations", sendDraft, Ask},
		{"mutations", listMessages, Allow},
		{"reads", listMessages, Ask},
		{"reads", sendDraft, Allow},
		{"deletions", deleteMessages, Ask},
		{"deletions", sendDraft, Allow},
		{"all", listMessages, Ask},
		{"all", deleteMessages, Ask},
		{"default", deleteMessages, Allow},
	} {
		if got := policy(t, tc.policy).Require(tc.cmd).Outcome; got != tc.want {
			t.Errorf("%q on %v = %v, want %v", tc.policy, tc.cmd.Path, got, tc.want)
		}
	}
}

// A scope is a prefix of the command, so naming an app covers everything in it
// and naming a command covers only that one.
func TestScopeReachesAsFarAsItIsWritten(t *testing.T) {
	for _, tc := range []struct {
		policy string
		cmd    Command
		want   Outcome
	}{
		{"mail:all", listMessages, Ask},
		{"mail:all", listDriveItems, Allow},
		{"mail messages:all", listMessages, Ask},
		{"mail drafts:all", listMessages, Allow},
		{"mail drafts send:all", sendDraft, Ask},
		{"mail drafts send:all", listMessages, Allow},
	} {
		if got := policy(t, tc.policy).Require(tc.cmd).Outcome; got != tc.want {
			t.Errorf("%q on %v = %v, want %v", tc.policy, tc.cmd.Path, got, tc.want)
		}
	}
}

// The narrowest scope that mentions a command decides, which is the only way one
// directive can carve an exception out of another.
func TestNarrowestScopeDecides(t *testing.T) {
	p := policy(t, "mutations, drive:default")
	if got := p.Require(sendDraft).Outcome; got != Ask {
		t.Errorf("outside the exception = %v, want Ask", got)
	}
	if got := p.Require(deleteDriveItem).Outcome; got != Allow {
		t.Errorf("inside the exception = %v, want Allow", got)
	}
}

// Deny is settled before ask and without regard to how narrowly each was
// written, so no directive can be placed in front of a refusal to soften it.
func TestDenyIsNotSoftenedByANarrowerAsk(t *testing.T) {
	for _, src := range []string{
		"deletions=deny, mail messages delete:all",
		"mail messages delete:all, deletions=deny",
		"mail:all, drive:all=deny",
	} {
		p := policy(t, src)
		cmd := deleteMessages
		if src == "mail:all, drive:all=deny" {
			cmd = deleteDriveItem
		}
		if got := p.Require(cmd).Outcome; got != Deny {
			t.Errorf("%q = %v, want Deny", src, got)
		}
	}
}

// Sources concatenate rather than replace, so the policy only ever ratchets:
// whichever of them is most cautious about a command is the one that decides.
func TestSourcesOnlyEverTighten(t *testing.T) {
	if got := policy(t, "mutations", "drive:default").Require(deleteDriveItem).Outcome; got != Ask {
		t.Errorf("a later lenient source must not loosen an earlier strict one: got %v", got)
	}
	if got := policy(t, "drive:default", "mutations").Require(deleteDriveItem).Outcome; got != Ask {
		t.Errorf("order between sources must not matter: got %v", got)
	}
	// Scoping still works inside the source that wrote it.
	if got := policy(t, "mutations, drive:default").Require(deleteDriveItem).Outcome; got != Allow {
		t.Errorf("an exception written beside its rule must hold: got %v", got)
	}
}

// An empty policy is the ordinary case, and it demands nothing.
func TestNoPolicyDemandsNothing(t *testing.T) {
	for _, cmd := range []Command{deleteMessages, sendDraft, listMessages} {
		if got := (Policy{}).Require(cmd).Outcome; got != Allow {
			t.Errorf("%v under no policy = %v, want Allow", cmd.Path, got)
		}
	}
}

// The class that decided is carried back, because a refusal that cannot say
// which rule caught it leaves the reader guessing at their own configuration.
func TestDecisionNamesTheClassThatDecided(t *testing.T) {
	if got := policy(t, "deletions=deny").Require(deleteMessages); got.Class != Deletions {
		t.Errorf("class = %v, want deletions", got.Class)
	}
	if got := policy(t, "reads").Require(listMessages); got.Class != Reads {
		t.Errorf("class = %v, want reads", got.Class)
	}
}

// All has no subject of its own: a policy covering every command has nothing
// narrower to say than the command that was run.
func TestOnlyAllHasNoSubject(t *testing.T) {
	for _, c := range Classes {
		subject := c.Subject()
		if (c == All || c == Default) != (subject == "") {
			t.Errorf("%v subject %q", c, subject)
		}
	}
}
