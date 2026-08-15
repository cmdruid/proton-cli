package cli

import (
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/roman-16/proton-cli/internal/cli/kit"
	"github.com/roman-16/proton-cli/internal/ui"
	"github.com/spf13/pflag"
)

const (
	dashedID    = "-bJxDLEMvt-Z6t4Yna7V8SYQ_FIHWT2_QbBr-whe-bIE8rbZunzr5RhXGaihvQ43z2qcxcqFgVRwi7A=="
	plainID     = "NWM5AYGxFIHWT2_QbBr-whe-bIE8rbZunzr5RhXGaihvQ43z2qcxcqFgVRwi7A5C-ADmohv7TjXfYbDEIHZPQ=="
	shortDashed = "-abc=="
)

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

// ── leading-dash references ──
//
// Proton's IDs are base64, '-' is one of its sixty-four characters, and so about
// one ID in sixty-four begins with a dash. Every one of those is a reference the
// CLI printed and argv hands to the flag parser first.

// These are real IDs, taken from a live account, not invented. A test written
// against invented IDs is a test written against the same assumptions as the
// code it checks.
const (
	realVault     = "-x76EpiVSJf2oHzHgyC2D_jF8Oi0yWMKsQdUvh1axN5Xx2bDFGUd-4ArpN5CZZrPXRvP6aMQjV8cgTDEvXBRQw=="
	realItem      = "_fb26gvMWjnM7US4_wpTNm_LqIAx5LJk9c0Rj7dUR4eobSmnTl1qNsiczzmIpgio08-67uw8sneSwGLPwkN5Vw=="
	realDriveLink = "-Qt-s7R_oGCru5u3Kv6Y8Q"
)

// The promise: a reference the CLI prints is one it will read back.
//
// This is the property, stated end to end rather than in pieces. Every reference
// is rendered exactly as a listing renders it - shortened for a terminal, full
// for a pipe - and then has to survive being typed back as the next command's
// argument.
//
// It failed in both forms before. `pass items list` printed SHARE/ITEM rows that
// answered "Unknown shorthand flag: 'x'", and a shortened Drive link did the
// same, because deciding by shape alone cannot tell "-Qt-s7R_" from a run of
// eight shorthand flags.
func TestEveryReferenceTheCLIPrintsCanBeTypedBackIn(t *testing.T) {
	for _, tc := range []struct{ name, command, reference string }{
		{"a Pass item", "pass items get", ui.Short(kit.JoinPair(realVault, realItem), false)},
		{"a Pass item, shortened", "pass items get", ui.Short(kit.JoinPair(realVault, realItem), true)},
		{"a Drive link", "drive photos download", realDriveLink},
		{"a Drive link, shortened", "drive photos download", ui.Short(realDriveLink, true)},
		{"an event occurrence", "calendar events get", ui.Short(realVault+"/"+realItem+"@2026-04-22T09:00", true)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			argv := append([]string{"proton-cli"}, strings.Fields(tc.command)...)
			argv = append(argv, tc.reference)

			cmd, rest, err := newRoot().Find(preprocessArgs(argv)[1:])
			if err != nil {
				t.Fatalf("%q: %v", tc.reference, err)
			}
			if err := cmd.ParseFlags(rest); err != nil {
				t.Fatalf("%q could not be typed back in: %v", tc.reference, err)
			}
			if got := cmd.Flags().Args(); len(got) != 1 || got[0] != tc.reference {
				t.Errorf("%q arrived as %q", tc.reference, got)
			}
		})
	}
}

// A reference that is a flag's value was never in danger: pflag reads the token
// after a value-taking flag whatever it starts with. Inserting "--" there is what
// breaks it, handing "--" to the flag and stranding the reference.
func TestADashedReferenceReachesTheFlagItWasGivenTo(t *testing.T) {
	argv := preprocessArgs([]string{
		"proton-cli", "drive", "photos", "list", "--album", realDriveLink,
	})
	cmd, rest, err := newRoot().Find(argv[1:])
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.ParseFlags(rest); err != nil {
		t.Fatal(err)
	}
	if got := cmd.Flags().Lookup("album").Value.String(); got != realDriveLink {
		t.Errorf("--album = %q, want the ID", got)
	}
}

// After a boolean flag, a reference really is positional and really does need
// protecting.
func TestPreprocessArgsStillProtectsAPositionalAfterABoolFlag(t *testing.T) {
	in := []string{"proton-cli", "mail", "messages", "trash", "--unread", realDriveLink}
	want := []string{"proton-cli", "mail", "messages", "trash", "--unread", "--", realDriveLink}
	if got := preprocessArgs(in); !equalSlice(got, want) {
		t.Errorf("preprocessArgs left a positional unprotected:\n got %v\nwant %v", got, want)
	}
}

// A token that really does name shorthand flags this command has must stay
// flags. That reading wins over "it looks like a reference", and it is the one
// thing the length heuristics could never get right.
func TestPreprocessArgsLeavesRealShorthandFlagsAlone(t *testing.T) {
	for _, token := range []string{"-o", "-ojson", "-h", "-v"} {
		in := []string{"proton-cli", "mail", "messages", "list", token}
		if got := preprocessArgs(in); !equalSlice(got, in) {
			t.Errorf("preprocessArgs rewrote the flags %q: %v", token, got)
		}
	}
}
