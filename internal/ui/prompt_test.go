package ui

import (
	"strings"
	"testing"
)

// Presence is what counts, whatever the value - the same rule NO_COLOR follows,
// so the two variables need only one mental model between them.
func TestNoInputCountsPresenceNotValue(t *testing.T) {
	for _, value := range []string{"1", "true", "0", "false", "no", "", " ", "anything"} {
		t.Setenv("PROTON_NO_INPUT", value)
		if !NoInput() {
			t.Errorf("PROTON_NO_INPUT=%q is set, so prompting is forbidden", value)
		}
	}
}

func TestNoInputUnsetIsFalse(t *testing.T) {
	unsetenv(t, "PROTON_NO_INPUT")
	if NoInput() {
		t.Error("an unset variable should not forbid prompting")
	}
}

// The environment and the flag are two ways to say the same thing, so either
// alone is enough.
func TestCanPromptRespectsBothTheFlagAndTheEnvironment(t *testing.T) {
	t.Run("the flag alone", func(t *testing.T) {
		unsetenv(t, "PROTON_NO_INPUT")
		u, _, _ := fixture(t, Options{NoInput: true})
		if u.CanPrompt() {
			t.Error("--no-input should forbid prompting")
		}
	})
	t.Run("the environment alone", func(t *testing.T) {
		t.Setenv("PROTON_NO_INPUT", "")
		u, _, _ := fixture(t, Options{})
		if u.CanPrompt() {
			t.Error("PROTON_NO_INPUT should forbid prompting")
		}
	})
	t.Run("neither, but stdin is not a terminal", func(t *testing.T) {
		unsetenv(t, "PROTON_NO_INPUT")
		u, _, _ := fixture(t, Options{})
		// The test process has no terminal on stdin, so prompting is impossible
		// regardless - which is the property a cron job relies on.
		if u.CanPrompt() {
			t.Error("a non-terminal stdin cannot be asked a question")
		}
	})
}

// The questions a sign-in asks form one block, so they are measured like one.
// Unaligned labels put every answer at a different column and make the block
// read as three unrelated questions rather than one form.
func TestAskAlignsTheLabelsItWasGiven(t *testing.T) {
	unsetenv(t, "PROTON_NO_INPUT")
	u, _, errb := fixture(t, Options{In: strings.NewReader("you@proton.me\n123456\n")})
	p := u.Ask("Email", "Password", "Two-factor code")
	if _, err := p.Line("Email"); err != nil {
		t.Fatal(err)
	}
	if _, err := p.Line("Two-factor code"); err != nil {
		t.Fatal(err)
	}
	want := "Email:            Two-factor code:  "
	if got := errb.String(); got != want {
		t.Errorf("prompts are not aligned\n got %q\nwant %q", got, want)
	}
}

// One reader for the whole block is what keeps a value typed before its question
// was asked. A reader per question buffers ahead and then throws the buffer away.
func TestAskKeepsWhatWasTypedAhead(t *testing.T) {
	unsetenv(t, "PROTON_NO_INPUT")
	u, _, _ := fixture(t, Options{In: strings.NewReader("you@proton.me\n123456\n")})
	p := u.Ask("Email", "Two-factor code")
	if got, _ := p.Line("Email"); got != "you@proton.me" {
		t.Fatalf("first answer = %q", got)
	}
	if got, _ := p.Line("Two-factor code"); got != "123456" {
		t.Errorf("second answer = %q, want the line typed ahead", got)
	}
}

// Refusing to prompt has to produce an error a reader can act on, naming both
// ways to supply the value.
func TestPromptRefusalIsActionable(t *testing.T) {
	t.Setenv("PROTON_NO_INPUT", "1")
	u, _, errb := fixture(t, Options{})
	if _, err := u.Ask().Secret("Password"); err == nil {
		t.Fatal("want a refusal")
	}
	if errb.Len() != 0 {
		t.Errorf("a refused prompt should print nothing, got %q", errb.String())
	}
}
