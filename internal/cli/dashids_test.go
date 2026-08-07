package cli

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
	shortDashed = "-abc=="
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
			if got := looksLikeDashedProtonID(tc.in); got != tc.want {
				t.Errorf("looksLikeDashedProtonID(%q) = %v, want %v", tc.in, got, tc.want)
			}
		})
	}
}

func TestPreprocessArgs(t *testing.T) {
	t.Run("plain ID untouched", func(t *testing.T) {
		in := []string{"proton-cli", "mail", "messages", "read", plainID}
		if got := preprocessArgs(in); len(got) != len(in) {
			t.Fatalf("preprocessArgs grew args: %v -> %v", in, got)
		}
	})
	t.Run("dashed ID gets -- inserted", func(t *testing.T) {
		in := []string{"proton-cli", "mail", "messages", "read", dashedID}
		want := []string{"proton-cli", "mail", "messages", "read", "--", dashedID}
		if got := preprocessArgs(in); !equalSlice(got, want) {
			t.Errorf("got %v, want %v", got, want)
		}
	})
	t.Run("dashed ID after flag value", func(t *testing.T) {
		in := []string{"proton-cli", "mail", "messages", "read", "--format", "raw", dashedID}
		want := []string{"proton-cli", "mail", "messages", "read", "--format", "raw", "--", dashedID}
		if got := preprocessArgs(in); !equalSlice(got, want) {
			t.Errorf("got %v, want %v", got, want)
		}
	})
	t.Run("only first dashed ID gets --, rest protected by terminator", func(t *testing.T) {
		in := []string{"proton-cli", "mail", "messages", "trash", dashedID, dashedID}
		want := []string{"proton-cli", "mail", "messages", "trash", "--", dashedID, dashedID}
		if got := preprocessArgs(in); !equalSlice(got, want) {
			t.Errorf("got %v, want %v", got, want)
		}
	})
	t.Run("user already inserted -- leaves args alone", func(t *testing.T) {
		in := []string{"proton-cli", "mail", "messages", "read", "--", dashedID}
		if got := preprocessArgs(in); !equalSlice(got, in) {
			t.Errorf("preprocessArgs altered args after user --: got %v, want %v", got, in)
		}
	})
	t.Run("real flag never matches", func(t *testing.T) {
		in := []string{"proton-cli", "--verbose", "mail", "messages", "list"}
		if got := preprocessArgs(in); !equalSlice(got, in) {
			t.Errorf("preprocessArgs altered args with real flag: got %v, want %v", got, in)
		}
	})
}

func TestRewrapFlagError(t *testing.T) {
	t.Run("non-pflag error passes through", func(t *testing.T) {
		err := errors.New("some other error")
		if got := rewrapFlagError(err, []string{"proton-cli"}); got != err {
			t.Errorf("rewrapFlagError altered non-pflag error: got %v", got)
		}
	})
	t.Run("nil passes through", func(t *testing.T) {
		if rewrapFlagError(nil, []string{"proton-cli"}) != nil {
			t.Error("rewrapFlagError(nil) != nil")
		}
	})
	t.Run("pflag shorthand error with ID-shape token rewraps", func(t *testing.T) {
		fs := pflag.NewFlagSet("test", pflag.ContinueOnError)
		fs.SetOutput(io.Discard)
		err := fs.Parse([]string{dashedID})
		if err == nil {
			t.Fatal("expected pflag parse error")
		}
		gotMsg := rewrapFlagError(err, []string{"proton-cli"}).Error()
		if !strings.Contains(gotMsg, "looks like a flag") || !strings.Contains(gotMsg, "insert -- before it") || !strings.Contains(gotMsg, dashedID) {
			t.Errorf("unexpected rewrapped message: %s", gotMsg)
		}
	})
	t.Run("pflag shorthand error with non-ID token passes through", func(t *testing.T) {
		fs := pflag.NewFlagSet("test", pflag.ContinueOnError)
		fs.SetOutput(io.Discard)
		err := fs.Parse([]string{"-xyz"})
		if err == nil {
			t.Fatal("expected pflag parse error")
		}
		if strings.Contains(rewrapFlagError(err, []string{"proton-cli"}).Error(), "looks like a flag because") {
			t.Error("rewrap fired on non-ID token")
		}
	})
	t.Run("cobra accepts-N-args error with dashed ID rewraps", func(t *testing.T) {
		err := errors.New("accepts 1 arg(s), received 3")
		argv := []string{"proton-cli", "mail", "messages", "read", dashedID, "--format", "raw"}
		gotMsg := rewrapFlagError(err, argv).Error()
		if !strings.Contains(gotMsg, "insert -- before it") || !strings.Contains(gotMsg, "Put flags") || !strings.Contains(gotMsg, dashedID) {
			t.Errorf("unexpected message: %s", gotMsg)
		}
	})
	t.Run("cobra accepts-N-args error without dashed ID passes through", func(t *testing.T) {
		err := errors.New("accepts 1 arg(s), received 3")
		argv := []string{"proton-cli", "mail", "messages", "read", plainID, "--format", "raw"}
		if strings.Contains(rewrapFlagError(err, argv).Error(), "insert -- before it") {
			t.Error("rewrap fired without dashed ID")
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

// An unrecognised flag is reported as a flag, wherever it appears. Cobra's
// default is to give up routing and blame the subcommand, which sends a reader
// looking for a command that is right there.
func TestUnknownFlagIsReportedAsAFlag(t *testing.T) {
	for _, args := range [][]string{
		{"--verbose", "account", "get"},
		{"account", "--verbose", "get"},
		{"account", "get", "--verbose"},
	} {
		root := newRoot()
		root.SetArgs(args)
		root.SetOut(io.Discard)
		root.SetErr(io.Discard)
		err := root.Execute()
		if err == nil {
			t.Errorf("%v: want an error", args)
			continue
		}
		if !strings.Contains(err.Error(), "unknown flag") {
			t.Errorf("%v: error %q should name the flag", args, err)
		}
	}
}
