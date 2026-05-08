package cmd

import (
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/spf13/pflag"
)

const (
	dashedID    = "-bJxDLEMvt-Z6t4Yna7V8SYQ_FIHWT2_QbBr-whe-bIE8rbZunzr5RhXGaihvQ43z2qcxcqFgVRwi7A=="
	plainID     = "NWM5AYGxFIHWT2_QbBr-whe-bIE8rbZunzr5RhXGaihvQ43z2qcxcqFgVRwi7A5C-ADmohv7TjXfYbDEIHZPQ=="
	shortDashed = "-abc==" // dashed but too short
)

func TestLooksLikeDashedProtonID(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want bool
	}{
		{"canonical leading-dash ID", dashedID, true},
		{"plain ID without leading dash", plainID, false},
		{"verbose long flag", "--verbose", false},
		{"short flag", "-v", false},
		{"too short", shortDashed, false},
		{"missing == suffix", "-bJxDLEMvt-Z6t4Yna7V8SYQ_FIHWT2_QbBr-whe-bIE8rbZunzr5RhXGaihvQ43z2qcxcqFgVRwi7A", false},
		{"non-base64 chars", "-bJxDLEMvt-Z6t4Yna7V8SYQ_FIHWT2_QbBr!whe$bIE8rbZunzr5RhXGaihvQ43z2qcxcqFgVRwi7A==", false},
		{"empty", "", false},
		{"single dash", "-", false},
		{"flag with eq sign", "--name=John Doe Long Title Goes Here Lorem ipsum aaaaaaaa==", false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := looksLikeDashedProtonID(tc.in)
			if got != tc.want {
				t.Errorf("looksLikeDashedProtonID(%q) = %v, want %v", tc.in, got, tc.want)
			}
		})
	}
}

func TestPreprocessArgs(t *testing.T) {
	t.Run("plain ID untouched", func(t *testing.T) {
		in := []string{"proton-cli", "mail", "messages", "read", plainID}
		got := preprocessArgs(in)
		if len(got) != len(in) {
			t.Fatalf("preprocessArgs grew args: %v -> %v", in, got)
		}
	})

	t.Run("dashed ID gets -- inserted", func(t *testing.T) {
		in := []string{"proton-cli", "mail", "messages", "read", dashedID}
		got := preprocessArgs(in)
		want := []string{"proton-cli", "mail", "messages", "read", "--", dashedID}
		if !equalSlice(got, want) {
			t.Errorf("got %v, want %v", got, want)
		}
	})

	t.Run("dashed ID after flag value", func(t *testing.T) {
		in := []string{"proton-cli", "mail", "messages", "read", "--format", "raw", dashedID}
		got := preprocessArgs(in)
		want := []string{"proton-cli", "mail", "messages", "read", "--format", "raw", "--", dashedID}
		if !equalSlice(got, want) {
			t.Errorf("got %v, want %v", got, want)
		}
	})

	t.Run("only first dashed ID gets --, rest is protected by terminator", func(t *testing.T) {
		in := []string{"proton-cli", "mail", "messages", "trash", dashedID, dashedID}
		got := preprocessArgs(in)
		want := []string{"proton-cli", "mail", "messages", "trash", "--", dashedID, dashedID}
		if !equalSlice(got, want) {
			t.Errorf("got %v, want %v", got, want)
		}
	})

	t.Run("user already inserted -- leaves args alone", func(t *testing.T) {
		in := []string{"proton-cli", "mail", "messages", "read", "--", dashedID}
		got := preprocessArgs(in)
		if !equalSlice(got, in) {
			t.Errorf("preprocessArgs altered args after user --: got %v, want %v", got, in)
		}
	})

	t.Run("real flag never matches", func(t *testing.T) {
		in := []string{"proton-cli", "--verbose", "mail", "messages", "list"}
		got := preprocessArgs(in)
		if !equalSlice(got, in) {
			t.Errorf("preprocessArgs altered args with real flag: got %v, want %v", got, in)
		}
	})
}

func TestRewrapFlagError(t *testing.T) {
	t.Run("non-pflag error passes through", func(t *testing.T) {
		err := errors.New("some other error")
		got := rewrapFlagError(err, []string{"proton-cli"})
		if got != err {
			t.Errorf("rewrapFlagError altered non-pflag error: got %v", got)
		}
	})

	t.Run("nil passes through", func(t *testing.T) {
		if rewrapFlagError(nil, []string{"proton-cli"}) != nil {
			t.Error("rewrapFlagError(nil) != nil")
		}
	})

	t.Run("pflag shorthand error with ID-shape token rewraps", func(t *testing.T) {
		// Use pflag's actual error machinery to build a NotExistError that
		// matches what cobra would surface for a leading-dash ID.
		fs := pflag.NewFlagSet("test", pflag.ContinueOnError)
		fs.SetOutput(io.Discard) // suppress default printing
		err := fs.Parse([]string{dashedID})
		if err == nil {
			t.Fatal("expected pflag parse error")
		}
		got := rewrapFlagError(err, []string{"proton-cli"})
		gotMsg := got.Error()
		if !strings.Contains(gotMsg, "looks like a flag") {
			t.Errorf("expected rewrapped message, got: %s", gotMsg)
		}
		if !strings.Contains(gotMsg, "insert -- before it") {
			t.Errorf("expected hint about --, got: %s", gotMsg)
		}
		if !strings.Contains(gotMsg, dashedID) {
			t.Errorf("expected message to include the offending token, got: %s", gotMsg)
		}
	})

	t.Run("pflag shorthand error with non-ID token passes through", func(t *testing.T) {
		fs := pflag.NewFlagSet("test", pflag.ContinueOnError)
		fs.SetOutput(io.Discard)
		err := fs.Parse([]string{"-xyz"})
		if err == nil {
			t.Fatal("expected pflag parse error")
		}
		got := rewrapFlagError(err, []string{"proton-cli"})
		if strings.Contains(got.Error(), "looks like a flag because") {
			t.Errorf("rewrap fired on non-ID token: %s", got.Error())
		}
	})

	t.Run("cobra accepts-N-args error with dashed ID in argv rewraps", func(t *testing.T) {
		err := errors.New("accepts 1 arg(s), received 3")
		argv := []string{"proton-cli", "mail", "messages", "read", dashedID, "--format", "raw"}
		got := rewrapFlagError(err, argv)
		gotMsg := got.Error()
		if !strings.Contains(gotMsg, "insert -- before it") {
			t.Errorf("expected hint about --, got: %s", gotMsg)
		}
		if !strings.Contains(gotMsg, "Put flags") {
			t.Errorf("expected flag-ordering hint, got: %s", gotMsg)
		}
		if !strings.Contains(gotMsg, dashedID) {
			t.Errorf("expected the offending token, got: %s", gotMsg)
		}
	})

	t.Run("cobra accepts-N-args error without dashed ID passes through", func(t *testing.T) {
		err := errors.New("accepts 1 arg(s), received 3")
		argv := []string{"proton-cli", "mail", "messages", "read", plainID, "--format", "raw"}
		got := rewrapFlagError(err, argv)
		if strings.Contains(got.Error(), "insert -- before it") {
			t.Errorf("rewrap fired without dashed ID: %s", got.Error())
		}
	})
}

func equalSlice(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
