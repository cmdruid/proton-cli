package app

import (
	"bytes"
	"strings"
	"testing"

	"github.com/roman-16/proton-cli/internal/ui"
)

// An account with both a code and a key is asked once, as the code prompt: an
// answer is a code, an empty answer is the key. Everything else about the choice
// follows from what the run already has - a code passed as a flag, or nobody
// there to ask.
func TestWhichSecondFactorAnswers(t *testing.T) {
	for _, tc := range []struct {
		name     string
		typed    string
		flag     string
		noInput  bool
		alsoTOTP bool
		wantKey  bool
		wantCode string
	}{
		{name: "only a key is registered", alsoTOTP: false, wantKey: true},
		{name: "a code was typed", alsoTOTP: true, typed: "123456\n", wantCode: "123456"},
		{name: "the prompt was answered with a bare newline", alsoTOTP: true, typed: "\n", wantKey: true},
		{name: "a code came from --totp", alsoTOTP: true, flag: "654321", wantCode: "654321"},
		{name: "nobody to ask", alsoTOTP: true, noInput: true, wantCode: ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var out bytes.Buffer
			creds := newCredentials(ui.New(ui.Options{
				Format: ui.FormatText, Err: &out, Out: &out,
				In: strings.NewReader(tc.typed), NoInput: tc.noInput,
			}), "")
			creds.flagTOTP = tc.flag

			if got := creds.prefersSecurityKey(tc.alsoTOTP); got != tc.wantKey {
				t.Fatalf("prefersSecurityKey = %v, want %v", got, tc.wantKey)
			}
			if tc.wantKey {
				return
			}
			// Whatever was typed at that one prompt is the code, and is not asked
			// for a second time.
			code, err := creds.TOTP()
			if tc.wantCode == "" {
				if err == nil {
					t.Errorf("a run with nothing to answer with returned %q", code)
				}
				return
			}
			if err != nil || code != tc.wantCode {
				t.Errorf("TOTP = %q, %v; want %q", code, err, tc.wantCode)
			}
		})
	}
}

// The key is only worth mentioning where it is a choice: an account that has
// nothing else says nothing, because there is nothing to choose.
func TestTheKeyIsOfferedOnlyWhereItIsAChoice(t *testing.T) {
	said := func(alsoTOTP bool, typed string) string {
		var out bytes.Buffer
		creds := newCredentials(ui.New(ui.Options{
			Format: ui.FormatText, Err: &out, Out: &out, In: strings.NewReader(typed),
		}), "")
		creds.prefersSecurityKey(alsoTOTP)
		return out.String()
	}
	if got := said(true, "\n"); !strings.Contains(got, "security key") {
		t.Errorf("prompt = %q, want the key offered as the other way in", got)
	}
	if got := said(false, ""); got != "" {
		t.Errorf("prompt = %q, want nothing said where there is no choice", got)
	}
}
