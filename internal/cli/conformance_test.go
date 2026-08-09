package cli

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"testing"
	"unicode"

	"github.com/roman-16/proton-cli/internal/cli/kit"
	"github.com/roman-16/proton-cli/internal/ui"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// This file is the interface's specification, expressed as assertions rather
// than as prose.
//
// The CLI's inconsistencies did not arrive through carelessness; they arrived
// because nothing could tell that a new command had picked a different word for
// an existing idea. Every rule the design rests on is checked here, over the
// whole tree, with no network and no credentials, so a divergence fails a test
// instead of shipping.
//
// Rules that the tree does not satisfy yet are skipped with the step that turns
// them on. A skip is a debt with an address.

// ── walking the tree ──

// leaves returns every command that does work, and groups returns every command
// that only holds others.
func partition(t *testing.T) (leaves, groups []*cobra.Command) {
	t.Helper()
	var walk func(*cobra.Command)
	walk = func(c *cobra.Command) {
		if c.Hidden || c.Name() == "help" || c.Name() == "completion" {
			return
		}
		if c.HasSubCommands() {
			groups = append(groups, c)
		}
		if c.Runnable() {
			leaves = append(leaves, c)
		}
		for _, sub := range c.Commands() {
			walk(sub)
		}
	}
	walk(newRoot())
	return leaves, groups
}

func cmdPath(c *cobra.Command) string { return c.CommandPath() }

// ── rule 1: every leaf introduces itself the same way ──

func TestEveryLeafHasAShortInTheHouseStyle(t *testing.T) {
	leaves, _ := partition(t)
	if len(leaves) == 0 {
		t.Fatal("no leaves found; the tree walk is broken")
	}
	for _, c := range leaves {
		short := c.Short
		switch {
		case short == "":
			t.Errorf("%s: no Short; every command has to say what it does", cmdPath(c))
		case strings.HasSuffix(short, "."):
			t.Errorf("%s: Short ends in a period: %q", cmdPath(c), short)
		case !unicode.IsUpper(rune(short[0])):
			t.Errorf("%s: Short should open with a capital: %q", cmdPath(c), short)
		}
	}
}

func TestEveryGroupHasAShort(t *testing.T) {
	_, groups := partition(t)
	for _, c := range groups {
		if c.Short == "" && c.Name() != "proton-cli" {
			t.Errorf("%s: no Short", cmdPath(c))
		}
	}
}

// ── rule 2: groups never act ──

func TestGroupsNeverAct(t *testing.T) {
	_, groups := partition(t)
	for _, c := range groups {
		if c.Runnable() {
			t.Errorf("%s is a group but also runs; move the behaviour to a verb such as `get`", cmdPath(c))
		}
	}
}

// A group holds commands, so a word it does not hold is a mistake worth
// reporting. Cobra makes that check only at the root; unknownSubcommand makes
// it everywhere, and this is what says so for every group in the tree.
func TestGroupsRejectAnUnknownSubcommand(t *testing.T) {
	_, groups := partition(t)
	for _, c := range groups {
		path := strings.Fields(c.CommandPath())[1:]
		if err := unknownSubcommand(newRoot(), append(append([]string{}, path...), "nope")); err == nil {
			t.Errorf("%s: takes an unknown subcommand without complaint", cmdPath(c))
		}
		if err := unknownSubcommand(newRoot(), path); err != nil {
			t.Errorf("%s: rejects being called on its own: %v", cmdPath(c), err)
		}
	}
}

// ── rule 3: one placeholder set ──

// placeholders are the only argument names the CLI uses. A new one means a new
// idea, which is exactly the thing worth noticing.
var placeholders = map[string]bool{
	"REF": true, "PATH": true, "SRC": true, "DEST": true, "EMAIL": true,
	"NEW_NAME": true, "KEY": true, "VALUE": true, "METHOD": true, "ENDPOINT": true,
	"ATTACHMENT_REF": true, "REVISION_REF": true, "CONTACT_REF": true, "PHOTO_REF": true,
}

func TestUsageUsesOnlyDeclaredPlaceholders(t *testing.T) {
	leaves, _ := partition(t)
	for _, c := range leaves {
		for _, tok := range argTokens(c.Use) {
			if !placeholders[tok] {
				t.Errorf("%s: undeclared placeholder %q in %q", cmdPath(c), tok, c.Use)
			}
		}
	}
}

// argTokens extracts the argument names from a Use string, stripping the command
// name and the optional/variadic decorations.
func argTokens(use string) []string {
	var out []string
	fields := strings.Fields(use)
	for _, f := range fields[min(1, len(fields)):] {
		f = strings.Trim(f, "[]{}()")
		f = strings.TrimSuffix(f, "...")
		if f == "" || f == "|" {
			continue
		}
		// Only shouted tokens are placeholders; a literal subcommand word is not.
		if f == strings.ToUpper(f) && strings.ContainsFunc(f, unicode.IsLetter) {
			out = append(out, f)
		}
	}
	return out
}

// ── rule 8: a flag name means one thing ──

// flagMeanings is the registry of every flag used by more than one command. Two
// commands may share a name only if they share the meaning.
//
// This is the rule that would have caught --all meaning four different things
// and --html meaning three.
var flagMeanings = map[string]string{
	"address":          "a postal or street address",
	"after":            "only things after a date",
	"album":            "the photo album to act in",
	"all":              "confirm that a filter-less selection really means everything in scope",
	"all-day":          "an event with no time of day",
	"attach":           "a file to attach",
	"attach-inline":    "an image to embed in an HTML body by Content-ID",
	"attendee":         "someone invited",
	"bcc":              "a blind-carbon-copy recipient",
	"before":           "only things before a date",
	"birthdate":        "a date of birth",
	"birthday":         "a date of birth",
	"body":             "the message body",
	"body-only":        "emit only the body",
	"calendar":         "the calendar to act in",
	"cc":               "a carbon-copy recipient",
	"check":            "report without changing anything",
	"city":             "a city",
	"clear-signature":  "remove the signature",
	"color":            "the accent colour to set",
	"country":          "a country",
	"cvv":              "a payment card verification number",
	"days":             "the days a schedule is active",
	"delete-photos":    "also remove the photos an album held",
	"description":      "free-text description",
	"detach":           "an attachment to remove",
	"disabled":         "create without turning it on",
	"display-name":     "the name recipients see",
	"draft":            "save instead of sending",
	"duration":         "how long something lasts",
	"edit":             "grant edit rather than view access",
	"email":            "an email address",
	"eml":              "an RFC 822 file to build the message from",
	"end":              "the end of a range or event",
	"eo-password":      "password-protect the message for recipients outside Proton",
	"eo-password-hint": "hint shown to password-protected recipients",
	"expires":          "how long before it stops working",
	"expiry":           "a payment card expiry date",
	"field":            "a custom field, as NAME=VALUE",
	"first-name":       "a given name",
	"folder":           "the mail location to look in",
	"force":            "overwrite a local file that already exists",
	"format":           "the shape of the output",
	"from":             "the sender: compose sets it, a filter matches it",
	"full-name":        "a full name",
	"hidden":           "a hidden custom field, as NAME=VALUE",
	"holder":           "the name on a payment card",
	"html":             "treat the text as HTML rather than escaping it",
	"include-inline":   "include inline attachments",
	"into":             "the destination container of a move or copy",
	"job-title":        "a job title",
	"key":              "an armoured PGP key",
	"keyword":          "full-text search term",
	"label":            "the label to attach or detach",
	"larger-than":      "select files above a size",
	"last-name":        "a family name",
	"limit":            "cap how many things are selected",
	"location":         "where something is",
	"mailbox":          "where mail to an alias should arrive",
	"message":          "an accompanying note",
	"name":             "the name to set",
	"newer-than":       "select things newer than a duration",
	"no-attachments":   "leave attachments out",
	"no-quote":         "do not quote the message being answered",
	"no-signature":     "leave the signature out",
	"note":             "free-text note",
	"number":           "a payment card number",
	"older-than":       "select things older than a duration",
	"organization":     "an organization name",
	"others":           "act on every session but this one",
	"output":           "where to write the payload; - is stdout",
	"output-dir":       "a directory to fill, keeping each item's own name",
	"page":             "which page of results",
	"page-size":        "how many results per page",
	"parent":           "the containing folder",
	"password":         "a password",
	"password-file":    "where to read the account password from",
	"password-stdin":   "read the account password from stdin",
	"pattern":          "select by glob against the name",
	"phone":            "a phone number",
	"pin":              "a payment card PIN",
	"postal-code":      "a postal code",
	"prefix":           "the local part of an alias",
	"private-key":      "a private key",
	"public-key":       "a public key",
	"purge":            "also remove local data",
	"query":            "a URL query parameter",
	"recursive":        "descend into subdirectories",
	"reinstall":        "install again even if already current",
	"remind":           "a reminder before the start",
	"repeat":           "how a schedule repeats",
	"revoke":           "also invalidate the session at Proton",
	"rrule":            "an iCalendar recurrence rule",
	"scope":            "the Drive subtree to look in",
	"security":         "a Wi-Fi security protocol",
	"send-at":          "when to deliver",
	"sieve":            "a Sieve script",
	"smaller-than":     "select files below a size",
	"ssid":             "a Wi-Fi network name",
	"starred":          "select starred things",
	"start":            "the beginning of a range or event",
	"status":           "the state to set",
	"strip-quotes":     "drop quoted reply blocks",
	"subject":          "the subject line: compose sets it, a filter matches it",
	"suffix":           "the domain part of an alias",
	"summary":          "one line per item instead of the whole thing",
	"tag":              "the photo tag to select",
	"title":            "a title: an event's, or a person's job title",
	"to":               "an email recipient: compose sets one, a filter matches one",
	"totp":             "a two-factor code",
	"totp-uri":         "a TOTP URI or secret, stored on a Pass login",
	"type":             "the kind of thing to create or select",
	"unread":           "select unread things",
	"url":              "a URL",
	"username":         "a login username",
	"vault":            "the Pass vault to act in",
	"website":          "a website address",
	"yes":              "proceed without asking",
	"zone":             "an IANA time zone",
}

func TestSharedFlagNamesShareOneMeaning(t *testing.T) {
	leaves, _ := partition(t)
	users := map[string][]string{}
	for _, c := range leaves {
		c.LocalFlags().VisitAll(func(f *pflag.Flag) {
			users[f.Name] = append(users[f.Name], cmdPath(c))
		})
	}
	for name, cmds := range users {
		if len(cmds) < 2 {
			continue
		}
		if _, declared := flagMeanings[name]; !declared {
			t.Errorf("--%s is used by %d commands but has no declared meaning:\n  %s",
				name, len(cmds), strings.Join(cmds, "\n  "))
		}
	}
}

// ── rule 5: flag help reads like the rest of the CLI ──

func TestFlagUsageIsInTheHouseStyle(t *testing.T) {
	leaves, groups := partition(t)
	seen := map[string]bool{}
	for _, c := range append(leaves, groups...) {
		c.Flags().VisitAll(func(f *pflag.Flag) {
			key := f.Name + "\x00" + f.Usage
			if seen[key] {
				return
			}
			seen[key] = true
			switch {
			case f.Usage == "":
				t.Errorf("%s --%s: no usage text", cmdPath(c), f.Name)
			case strings.HasSuffix(f.Usage, "."):
				t.Errorf("%s --%s: usage ends in a period: %q", cmdPath(c), f.Name, f.Usage)
			case !unicode.IsUpper(rune(f.Usage[0])) && !strings.HasPrefix(f.Usage, "-"):
				t.Errorf("%s --%s: usage should open with a capital: %q", cmdPath(c), f.Name, f.Usage)
			}
			if f.Name != strings.ToLower(f.Name) || strings.Contains(f.Name, "_") {
				t.Errorf("%s --%s: flag names are kebab-case", cmdPath(c), f.Name)
			}
		})
	}
}

// ── rule 7: only the ui package writes to the process streams ──

func TestOnlyTheUIPackageTouchesTheProcessStreams(t *testing.T) {
	// Execute is the one place with nowhere else to write: it reports the failures
	// that happen before an App - and therefore a renderer - exists, such as a
	// flag that could not be parsed.
	allowed := map[string]bool{"../cli/root.go": true}

	offenders := grepGo(t, []string{"../cli", "../service"}, func(src string) bool {
		return strings.Contains(src, "os.Stdout") || strings.Contains(src, "os.Stderr")
	})
	for _, f := range offenders {
		if !allowed[f] {
			t.Errorf("%s writes to a process stream; render through internal/ui instead", f)
		}
	}
}

// ── rule 10: only one place may ask a person for a credential ──

// A command that reads stdin behind the user's back is a command that can hang a
// cron job. Keeping the ability in one file is what makes that checkable.
func TestOnlyOnePlaceReadsCredentialsFromStdin(t *testing.T) {
	allowed := map[string]bool{
		"../ui/prompt.go": true,
	}
	offenders := grepGo(t, []string{"../cli", "../service", "../app", "../proton", "../account"}, func(src string) bool {
		return strings.Contains(src, "term.ReadPassword")
	})
	for _, f := range offenders {
		if !allowed[f] {
			t.Errorf("%s reads a secret from stdin; that belongs in internal/ui/prompt.go", f)
		}
	}
}

// ── rule 13: no environment variable may name an account ──

// An account is attached to a profile by `account login` and nowhere else. The
// moment a credential can arrive through the environment as well, a command can
// act as an account nobody named on the command line, which is how a profile
// ends up quietly meaning a different one.
func TestNoEnvironmentVariableCarriesACredential(t *testing.T) {
	forbidden := []string{"PROTON_USER", "PROTON_PASSWORD", "PROTON_TOTP"}
	offenders := grepGo(t, []string{"../cli", "../app", "../service", "../account", "../proton"},
		func(src string) bool {
			for _, v := range forbidden {
				if strings.Contains(src, v) {
					return true
				}
			}
			return false
		})
	for _, f := range offenders {
		t.Errorf("%s takes an account from the environment; sign in with `account login` instead", f)
	}
}

// ── rule 14: standard input has one owner ──

// Two things want stdin: --password-stdin for the account password, and `-` for
// a body, a key, or a file to upload. Whichever read it second would find an
// empty stream and fail somewhere further along with a puzzle.
//
// So every reader goes through App.Stdin, which hands it out once and names both
// claimants when they collide. That is what lets --password-stdin be a global
// flag rather than a privilege one command holds: any command can be the one
// Proton asks to re-authenticate, and none of them has to know which.
func TestStandardInputHasOneOwner(t *testing.T) {
	// internal/ui owns the process streams (rule 7) and supplies the reader that
	// App.Stdin hands out.
	allowed := map[string]bool{"../ui/ui.go": true}
	offenders := grepGo(t, []string{"../cli", "../app", "../service", "../account", "../proton", "../ui"},
		func(src string) bool { return strings.Contains(src, "os.Stdin") })
	for _, f := range offenders {
		if !allowed[f] {
			t.Errorf("%s reads os.Stdin directly; go through App.Stdin so stdin keeps one owner", f)
		}
	}
}

// ── rule 15: the commands that can be asked to re-authenticate are declared ──

// Proton guards a few endpoints behind an elevated session and grants that only
// for another SRP exchange, so the commands reaching one carry the credentials
// to answer with. Which endpoints those are is Proton's to decide and not
// discoverable from here, so the set is written down: adding to it is a decision
// rather than a reflex, and the integration harness keeps the same list.
func TestReauthCommandsAreDeclared(t *testing.T) {
	want := []string{
		"proton-cli account login",
		"proton-cli calendar settings calendars delete",
		"proton-cli mail settings autoreply set",
	}
	leaves, _ := partition(t)
	var got []string
	for _, c := range leaves {
		if c.Flags().Lookup("password-file") != nil {
			got = append(got, cmdPath(c))
		}
	}
	sort.Strings(got)
	if !slices.Equal(got, want) {
		t.Errorf("commands carrying the credential flags are:\n  %s\nwant:\n  %s",
			strings.Join(got, "\n  "), strings.Join(want, "\n  "))
	}
}

// ── the vocabulary is closed ──

// The vocabulary is read from kit rather than restated here. A test that keeps
// its own copy of the list it is checking will eventually be checking the copy:
// this one had drifted to hold a verb the CLI no longer has.

func TestEveryVerbIsInTheVocabulary(t *testing.T) {
	leaves, _ := partition(t)
	for _, c := range leaves {
		if c.Name() == "proton-cli" {
			continue
		}
		if _, ok := kit.Verbs[c.Name()]; !ok {
			t.Errorf("%s: %q is not in the declared verb vocabulary", cmdPath(c), c.Name())
		}
	}
}

// ── rule 12: what cannot be taken back is declared, not remembered ──

// The CLI stops for a yes for one reason: something is about to be removed. The
// verbs that can never be taken back and the actions that say so have to name
// the same set, because the verb is what a user reads in the help and the action
// is what actually decides at run time. Two lists that can disagree are one bug
// away from a `delete` that never asks.
func TestIrreversibleVerbsAndActionsAgree(t *testing.T) {
	fromActions := map[string]bool{}
	for _, a := range ui.Actions {
		if a.Cost == ui.Forever {
			fromActions[a.Verb] = true
		}
	}
	for verb := range kit.Irreversible {
		if !fromActions[verb] {
			t.Errorf("%q is declared irreversible but no action reports it as Forever", verb)
		}
	}
	for verb := range fromActions {
		if !kit.Irreversible[verb] {
			t.Errorf("an action reports %q as Forever, but the verb is not declared irreversible", verb)
		}
	}
}

// Every leaf named by an irreversible verb has to be reachable as one, so a
// command cannot quietly sit outside the guard by being spelled differently.
func TestIrreversibleVerbsAreMutatingVerbs(t *testing.T) {
	for verb := range kit.Irreversible {
		if _, ok := kit.Verbs[verb]; !ok {
			t.Errorf("%q is declared irreversible but is not a verb", verb)
		}
		if !kit.Mutating[verb] {
			t.Errorf("%q is declared irreversible but is not declared mutating", verb)
		}
	}
}

// Whether a change happens at all is settled globally, so no command may
// redefine the two flags that settle it.
//
// A local --yes is not a naming clash to be tidied up; it is a command deciding
// for itself what consent means, which is the one thing the guard cannot
// survive. `uninstall` used to spell "actually do it" that way, leaving --yes
// meaning "proceed without asking" everywhere except the command with the most
// to lose.
func TestNoCommandRedefinesConsent(t *testing.T) {
	leaves, groups := partition(t)
	for _, c := range append(leaves, groups...) {
		for _, name := range []string{"yes", "dry-run"} {
			if f := c.Flags().Lookup(name); f != nil && c.InheritedFlags().Lookup(name) == nil {
				t.Errorf("%s declares its own --%s; that flag is the root's alone", cmdPath(c), name)
			}
		}
	}
}

// ── helpers ──

// grepGo returns the non-test Go files under the given directories whose source
// satisfies match, as paths relative to this package.
func grepGo(t *testing.T, dirs []string, match func(string) bool) []string {
	t.Helper()
	var hits []string
	for _, dir := range dirs {
		err := filepath.WalkDir(dir, func(p string, d os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() || !strings.HasSuffix(p, ".go") || strings.HasSuffix(p, "_test.go") {
				return nil
			}
			src, err := os.ReadFile(p)
			if err != nil {
				return err
			}
			// Parse so a mention inside a comment does not count as a use.
			fset := token.NewFileSet()
			file, perr := parser.ParseFile(fset, p, src, parser.SkipObjectResolution)
			if perr == nil {
				var b strings.Builder
				ast.Inspect(file, func(n ast.Node) bool {
					if sel, ok := n.(*ast.SelectorExpr); ok {
						if id, ok := sel.X.(*ast.Ident); ok {
							b.WriteString(id.Name + "." + sel.Sel.Name + "\n")
						}
					}
					return true
				})
				if match(b.String()) {
					hits = append(hits, filepath.ToSlash(p))
				}
				return nil
			}
			if match(string(src)) {
				hits = append(hits, filepath.ToSlash(p))
			}
			return nil
		})
		if err != nil {
			t.Fatalf("walk %s: %v", dir, err)
		}
	}
	return hits
}

// ── rule 11: layers import downward only ──

// layers records, per package, which of our own packages it may import.
//
// The direction is what keeps the design honest. An inversion - a domain service
// reaching for the progress bar, the presentation package holding mail body
// transforms - stays invisible until stated as a rule, and shows up as the same
// symptom either way: neither half can be tested without the other.
var layers = map[string][]string{
	"ui":       {"units", "progress", "errs"},
	"proton":   {"errs", "crypto/aead", "hv", "hv/hvexit"},
	"errs":     {},
	"units":    {},
	"progress": {},
	"mailtext": {},
	"idcache":  {},
	"ref":      {"errs"},
	"ical":     {},
}

func TestPackagesImportDownwardOnly(t *testing.T) {
	const mod = "github.com/roman-16/proton-cli/internal/"
	for pkg, allowed := range layers {
		permitted := map[string]bool{}
		for _, a := range allowed {
			permitted[a] = true
		}
		for _, imported := range ourImports(t, filepath.Join("..", pkg), mod) {
			if imported == pkg || strings.HasPrefix(imported, pkg+"/") {
				continue
			}
			if !permitted[imported] {
				t.Errorf("%s imports %s, which is not below it", pkg, imported)
			}
		}
	}
}

// ourImports lists the internal packages dir's non-test sources import.
func ourImports(t *testing.T, dir, prefix string) []string {
	t.Helper()
	seen := map[string]bool{}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read %s: %v", dir, err)
	}
	fset := token.NewFileSet()
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".go") || strings.HasSuffix(e.Name(), "_test.go") {
			continue
		}
		file, err := parser.ParseFile(fset, filepath.Join(dir, e.Name()), nil, parser.ImportsOnly)
		if err != nil {
			t.Fatalf("parse %s: %v", e.Name(), err)
		}
		for _, imp := range file.Imports {
			path := strings.Trim(imp.Path.Value, `"`)
			if after, ok := strings.CutPrefix(path, prefix); ok {
				seen[after] = true
			}
		}
	}
	out := make([]string, 0, len(seen))
	for p := range seen {
		out = append(out, p)
	}
	return out
}
