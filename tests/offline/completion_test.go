package offline

import (
	"strings"
	"testing"
)

// A shell completes by running the binary: every generated script calls
// `proton __complete <words>` and reads what comes back. So the completion
// request is part of the interface, it costs no account and no network, and it
// is checked here rather than trusted.
func TestTheShellCanAskWhatComesNext(t *testing.T) {
	for _, request := range []string{"__complete", "__completeNoDesc"} {
		stdout, stderr, code := run(t, request, "")
		if code != 0 {
			t.Errorf("%s: exit %d, want 0\nstderr: %s", request, code, truncate(stderr))
			continue
		}
		for _, want := range []string{"changelog", "drive", "mail", "update"} {
			if !strings.Contains(stdout, want) {
				t.Errorf("%s: does not offer %q\nstdout: %s", request, want, truncate(stdout))
			}
		}
	}
}

func TestTheShellCanAskWhatComesAfterAGroup(t *testing.T) {
	stdout, _, code := run(t, "__complete", "mail", "messages", "")
	if code != 0 {
		t.Fatalf("exit %d, want 0", code)
	}
	for _, want := range []string{"list", "send", "trash"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("does not offer %q\nstdout: %s", want, truncate(stdout))
		}
	}
}
