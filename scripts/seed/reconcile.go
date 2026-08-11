package main

import (
	"fmt"
	"path"
	"strings"
)

// A pin is one row a collection has to hold.
//
// It is judged on the fields named here and on nothing else: an ID and a
// timestamp are the server's to choose, so demanding those match would mean
// rebuilding the account on every run.
type pin struct {
	id     string            // the value identifying it, under the collection's key
	fields map[string]string // what else has to match
	create []string          // the command that makes it
}

// A collection is one list the fixture pins rows in.
type collection struct {
	what   string   // a noun for the report
	list   []string // the command that lists it
	key    string   // the field holding a row's identity
	idKeys []string // the fields holding a row's ID, joined into the reference a removal takes
	remove []string // the command that removes one, with the target appended
	// parent is set on a collection the CLI addresses by path rather than by ID:
	// Drive names a thing by where it sits in the tree.
	parent string
	pins   []pin
}

// target renders the reference c.remove takes for one row. An event needs two
// IDs to address it and is written the way the CLI writes it, which is why this
// is a join rather than a lookup.
func (c collection) target(row map[string]any, name string) string {
	if c.parent != "" {
		return path.Join(c.parent, name)
	}
	parts := make([]string, 0, len(c.idKeys))
	for _, k := range c.idKeys {
		parts = append(parts, str(row[k]))
	}
	return strings.Join(parts, "/")
}

// testPrefix is the namespace the suite makes its artifacts under.
const testPrefix = "proton-cli-test-"

// sweep removes what an interrupted suite run left behind.
//
// The suite clears up after itself; a run that was killed cannot, and what it
// leaves is indistinguishable from the account's own contents to everything
// except this prefix. It accumulates for as long as the accounts exist, it puts
// rows the fixture never declared in front of every list, and the README panel
// photographs whatever is there.
//
// A recurring event is listed once per occurrence and removed as a series, so a
// reference already swept is not swept again.
func (r *report) sweep(profile string, c collection, list []map[string]any) {
	swept := map[string]bool{}
	for _, row := range list {
		name := str(row[c.key])
		if !strings.HasPrefix(name, testPrefix) {
			continue
		}
		what := fmt.Sprintf("%s: %s", c.what, name)
		if len(c.remove) == 0 {
			r.fail(what, fmt.Errorf("left by the suite and cannot be removed"))
			continue
		}
		target := c.target(row, name)
		if swept[target] {
			continue
		}
		swept[target] = true
		if _, err := run(profile, append(append([]string{"--yes"}, c.remove...), target)...); err != nil {
			r.fail(what, err)
			continue
		}
		fmt.Printf("  - %s\n", what)
		r.swept++
	}
}

// reconcile brings one collection to the state the fixture declares.
//
// A row that is absent is made. A row that is present but disagrees with the
// fixture is removed and made again, because a half-right fixture is worse than
// a missing one: it passes a presence check and then fails an assertion
// somewhere far away. Rows the fixture says nothing about are left alone, unless
// they are the suite's own leftovers, which sweep takes.
func (r *report) reconcile(profile string, c collection) {
	list, err := rows(profile, c.list...)
	if err != nil {
		r.fail(c.what, err)
		return
	}
	r.sweep(profile, c, list)
	for _, p := range c.pins {
		what := fmt.Sprintf("%s: %s", c.what, p.id)
		row, found := find(list, c.key, p.id)
		switch {
		case !found:
			r.make(profile, what, p.create)
		case !agrees(row, p.fields):
			if len(c.remove) == 0 {
				r.fail(what, fmt.Errorf("does not match the fixture and cannot be replaced"))
				continue
			}
			if _, err := run(profile, append(append([]string{"--yes"}, c.remove...), c.target(row, p.id))...); err != nil {
				r.fail(what, err)
				continue
			}
			r.remake(profile, what, p.create)
		}
	}
}

// agrees reports whether a row matches every field the pin names.
func agrees(row map[string]any, fields map[string]string) bool {
	for k, want := range fields {
		if str(row[k]) != want {
			return false
		}
	}
	return true
}

// report is what the run did, so a seed that changed nothing can say so.
type report struct {
	made     int
	remade   int
	swept    int
	failures []string
}

func (r *report) make(profile, what string, args []string) {
	if _, err := run(profile, args...); err != nil {
		r.fail(what, err)
		return
	}
	fmt.Printf("  + %s\n", what)
	r.made++
}

func (r *report) remake(profile, what string, args []string) {
	if _, err := run(profile, args...); err != nil {
		r.fail(what, err)
		return
	}
	fmt.Printf("  ~ %s\n", what)
	r.remade++
}

func (r *report) fail(what string, err error) {
	fmt.Printf("  ! %s: %v\n", what, err)
	r.failures = append(r.failures, what)
}
