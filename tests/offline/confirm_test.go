package offline

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The confirmation policy is judged from the command line and the file alone, so
// every answer here costs a process and nothing else - no session, no network.

// withPolicy writes a config file and returns the arguments that point at it.
func withPolicy(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

// A denied command answers with its own exit code, because a caller has to tell
// a refusal from a mistake: nothing was wrong with the command, and repeating it
// differently will not help.
func TestADeniedCommandExitsSix(t *testing.T) {
	refuses(t, 6, []string{"--confirm", "deletions=deny", "mail", "messages", "delete", "5bH2mQxK"},
		"Deleting is turned off by your confirmation policy")
}

// Nothing answers a deny. --yes answers a question, and a deny is not one.
func TestYesDoesNotAnswerADeny(t *testing.T) {
	refuses(t, 6, []string{"--confirm", "deletions=deny", "--yes", "mail", "messages", "delete", "5bH2mQxK"},
		"turned off by your confirmation policy")
}

// A preview of a thing you may not do is still a claim that you may do it.
func TestDryRunDoesNotEscapeADeny(t *testing.T) {
	refuses(t, 6, []string{"--confirm", "drive:all=deny", "--dry-run", "drive", "items", "delete", "/x"},
		"turned off by your confirmation policy")
}

// The refusal carries no remedy. The reader is not stuck, they are stopped, and
// the sentence must not hand whatever ran the command the edit that lifts the
// guard.
func TestADenialOffersNoWayAroundItself(t *testing.T) {
	_, stderr, _ := run(t, "--confirm", "deletions=deny", "mail", "messages", "delete", "5bH2mQxK")
	for _, leak := range []string{"Try:", "config.yaml", "--yes", "deny"} {
		if strings.Contains(stderr, leak) {
			t.Errorf("the refusal mentions %q, which is a way around it:\n%s", leak, stderr)
		}
	}
}

// A command that reads has no filter to resolve and nothing to count, so it is
// stopped before it starts and names itself.
func TestAReadIsStoppedWhenThePolicyAsksAboutReads(t *testing.T) {
	refuses(t, 1, []string{"--confirm", "reads", "mail", "messages", "list"},
		"Running proton mail messages list needs confirmation",
		"Your confirmation policy asks about reads",
		"--yes to confirm")
}

// --yes answers an ask, wherever the ask came from. The command then fails for
// its own reasons, which here is having no account - the point is that it got
// past the policy.
func TestYesAnswersAPolicyAsk(t *testing.T) {
	_, stderr, code := run(t, "--confirm", "reads", "--yes", "mail", "messages", "list")
	if strings.Contains(stderr, "needs confirmation") {
		t.Errorf("--yes should have answered the policy:\n%s", stderr)
	}
	if code == 6 {
		t.Errorf("an ask is not a deny:\n%s", stderr)
	}
}

// A mutation waits for the place that can name what it would touch, so the
// policy does not turn a counted sentence into a guess.
func TestAMutationAsksWhereItCanSayWhatItWouldDo(t *testing.T) {
	_, stderr, _ := run(t, "--confirm", "mutations", "mail", "messages", "list")
	if strings.Contains(stderr, "needs confirmation") {
		t.Errorf("a read is not a mutation:\n%s", stderr)
	}
}

// The narrowest scope that mentions a command decides, within the source that
// wrote it.
func TestAScopedExceptionHoldsBesideItsRule(t *testing.T) {
	refuses(t, 1, []string{"--confirm", "reads, mail:default", "pass", "items", "list"},
		"needs confirmation")

	_, stderr, _ := run(t, "--confirm", "reads, mail:default", "mail", "messages", "list")
	if strings.Contains(stderr, "needs confirmation") {
		t.Errorf("the exception should have left mail alone:\n%s", stderr)
	}
}

// A flag is a source of its own, so it can add a requirement to what the file
// says and can never stand one down.
func TestAFlagCannotLoosenTheFile(t *testing.T) {
	path := withPolicy(t, "confirm:\n  deny:\n    \"*\": deletions\n")
	refuses(t, 6, []string{"--config", path, "--confirm", "default", "mail", "messages", "delete", "5bH2mQxK"},
		"turned off by your confirmation policy")
}

// A scope that names no command guards nothing, and a guard that is quietly
// absent is worse than one nobody wrote.
func TestAScopeThatIsNotACommandIsRefused(t *testing.T) {
	refuses(t, 1, []string{"--confirm", "mail lettuce:all", "mail", "messages", "list"},
		"is not a command, so it cannot be a confirmation scope")
}

// A class the policy invents is refused with the list of the ones there are.
func TestABadClassNamesTheRealOnes(t *testing.T) {
	refuses(t, 1, []string{"--confirm", "everything", "mail", "messages", "list"},
		"default", "deletions", "mutations", "reads", "all")
}

// Nothing runs on a file that does not parse: it carries the policy, and a
// policy that quietly fails to load is one that fails open.
func TestAMalformedConfigStopsEverything(t *testing.T) {
	path := withPolicy(t, "loglevel: warn\n")
	refuses(t, 1, []string{"--config", path, "mail", "messages", "list"}, "unknown field", path)
}

// A file somebody named and that is not there is a mistake worth reporting.
func TestANamedConfigThatIsMissingIsRefused(t *testing.T) {
	refuses(t, 1, []string{"--config", "/nonexistent/config.yaml", "mail", "messages", "list"},
		"/nonexistent/config.yaml")
}

// Which profile is in force decides which section applies, and only the top
// level may say it.
func TestAProfileInsideItsOwnSectionIsRefused(t *testing.T) {
	path := withPolicy(t, "per-profile:\n  work:\n    profile: other\n")
	refuses(t, 1, []string{"--config", path, "mail", "messages", "list"}, "unknown field", "profile")
}

// A per-profile section tightens the policy for that profile and leaves the
// others as they were.
func TestAPerProfileSectionAppliesOnlyToThatProfile(t *testing.T) {
	path := withPolicy(t, "per-profile:\n  work:\n    confirm:\n      deny:\n        \"*\": deletions\n")
	refuses(t, 6, []string{"--config", path, "--profile", "work", "mail", "messages", "delete", "5bH2mQxK"},
		"turned off by your confirmation policy")

	_, stderr, code := run(t, "--config", path, "--profile", "other", "mail", "messages", "delete", "5bH2mQxK", "--yes")
	if code == 6 || strings.Contains(stderr, "confirmation policy") {
		t.Errorf("another profile should be unaffected: exit %d\n%s", code, stderr)
	}
}

// With no policy written, every command behaves exactly as it always has.
func TestNoPolicyChangesNothing(t *testing.T) {
	_, stderr, code := run(t, "mail", "messages", "list")
	if code == 6 || strings.Contains(stderr, "confirmation policy") {
		t.Errorf("an unconfigured install must not consult a policy: exit %d\n%s", code, stderr)
	}
}
