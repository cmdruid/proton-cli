package tests

import (
	"fmt"
	"sync"
	"testing"

	"github.com/cmdruid/proton-cli/tests/fixture"
)

// What the account holds for the suite to read, brought about when something
// asks for it.
//
// A fixture is not made until a test needs it, so a run that touches no aliases
// makes no alias and one test costs one lookup rather than a whole account being
// filled first. Asking twice in a run costs nothing: each is found or made once
// and remembered.
//
// Only what the suite never changes belongs here. A listing remembered from
// before another test changed the thing would be a false pass, which is worse
// than a slow run - a test that mutates its fixture leases it and puts it back.

// suiteRunner is how the fixture package runs the CLI as one account.
var suiteRunner fixture.Runner = func(profile string, args ...string) (string, error) {
	out, stderr, code, err := runArgs(nil, append([]string{"--profile", profile}, args...)...)
	if err != nil {
		return "", err
	}
	if code != 0 {
		return "", fmt.Errorf("exit %d: %s", code, truncateOutput(stderr))
	}
	return out, nil
}

var (
	fixturesMu sync.Mutex
	fixtures   = map[string]*sync.Once{}
	fixtureRow = map[string]map[string]any{}
	fixtureErr = map[string]error{}
)

// pinned returns the row for one of the fixture's pins, making it if the account
// has not got it. The name is the one the fixture declares it under.
func pinned(t *testing.T, what, name string) map[string]any {
	t.Helper()
	c, p, ok := declaredPin(what, name)
	if !ok {
		t.Fatalf("nothing in the fixture called %q under %q", name, what)
	}
	key := what + "/" + name

	fixturesMu.Lock()
	once, seen := fixtures[key]
	if !seen {
		once = &sync.Once{}
		fixtures[key] = once
	}
	fixturesMu.Unlock()

	once.Do(func() {
		// What an interrupted run left behind is cleared from the listing this
		// is about to make anyway, so hygiene costs nothing where a fixture is
		// wanted. `just seed` sweeps everything, including collections this run
		// never looks at.
		if list, err := fixture.Rows(suiteRunner, primary, c.List...); err == nil {
			fixture.Sweep(suiteRunner, primary, c, list)
		}
		row, err := fixture.Ensure(suiteRunner, primary, c, p)
		fixturesMu.Lock()
		fixtureRow[key], fixtureErr[key] = row, err
		fixturesMu.Unlock()
	})

	fixturesMu.Lock()
	row, err := fixtureRow[key], fixtureErr[key]
	fixturesMu.Unlock()
	if err != nil {
		t.Fatalf("the account has no %s called %q and one could not be made: %v", what, name, err)
	}
	return row
}

// declaredPin finds a pin in the fixture, so a test names what it wants rather
// than repeating how to make it.
func declaredPin(what, name string) (fixture.Collection, fixture.Pin, bool) {
	for _, c := range fixture.Mailbox("") {
		if c.What != what {
			continue
		}
		if p, ok := c.Pin(name); ok {
			return c, p, true
		}
	}
	return fixture.Collection{}, fixture.Pin{}, false
}

// seededAlias is the alias the suite reads rather than makes.
//
// Making one is what Proton meters hardest here - a handful an hour, against
// several tests that each want one - so only the test about making an alias
// makes its own.
func seededAlias(t *testing.T) (ref, address string) {
	t.Helper()
	row := pinned(t, "pass item", fixture.AliasName)
	share, _ := row["share_id"].(string)
	id, _ := row["item_id"].(string)
	alias, _ := row["alias"].(string)
	return share + "/" + id, alias
}
