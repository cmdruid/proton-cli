package main

import (
	"fmt"
	"path"
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
	idKey  string   // the field holding a row's ID, for removal
	remove []string // the command that removes one, with the target appended
	// parent is set on a collection the CLI addresses by path rather than by ID:
	// Drive names a thing by where it sits in the tree.
	parent string
	pins   []pin
}

// reconcile brings one collection to the state the fixture declares.
//
// A row that is absent is made. A row that is present but disagrees with the
// fixture is removed and made again, because a half-right fixture is worse than
// a missing one: it passes a presence check and then fails an assertion
// somewhere far away. Rows the fixture says nothing about are left alone - the
// suite creates its own and cleans up after itself.
func (r *report) reconcile(profile string, c collection) {
	list, err := rows(profile, c.list...)
	if err != nil {
		r.fail(c.what, err)
		return
	}
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
			target := str(row[c.idKey])
			if c.parent != "" {
				target = path.Join(c.parent, p.id)
			}
			if _, err := run(profile, append(append([]string{"--yes"}, c.remove...), target)...); err != nil {
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
