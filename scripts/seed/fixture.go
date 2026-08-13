package main

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/roman-16/proton-cli/tests/fixture"
)

// The fixture. Both accounts hold the same of it, so a test can act as either
// and the README panel is a photograph of it.
//
// The names read like somebody's account rather than like fixtures for that
// second reason. Nothing here uses the `proton-cli-test-` prefix, which belongs
// to the artifacts the suite makes and clears up itself.

// panelMail is what the README panel shows, in the order it shows it.
var panelMail = []struct{ subject, body, attach string }{
	{"Invoice #2291 is ready", "Your invoice for this month is attached to your account.", ""},
	{"Your monthly security report", "No unusual sign-ins this month.", ""},
	{"Re: hiking weekend", "The north trail is open again - shall we take it?", "packing-list.txt"},
}

// files the fixture uploads, and their contents.
var files = map[string]string{
	"packing-list.txt": "Two tents, a stove, and the good map.\n",
	"trail-map.txt":    "The north trail is open again.\n",
	"panorama.jpg":     "",
}

func mailbox(work string) []collection {
	return []collection{{
		what:   "label",
		list:   []string{"mail", "settings", "labels", "list"},
		key:    "name",
		idKeys: []string{"id"},
		remove: []string{"mail", "settings", "labels", "delete"},
		pins: []pin{{
			id:     "Newsletters",
			fields: map[string]string{"color": "#8080FF"},
			create: []string{"mail", "settings", "labels", "create", "--name", "Newsletters", "--color", "#8080FF"},
		}},
	}, {
		what:   "folder",
		list:   []string{"mail", "settings", "folders", "list"},
		key:    "name",
		idKeys: []string{"id"},
		remove: []string{"mail", "settings", "folders", "delete"},
		pins: []pin{{
			id:     "Projects",
			fields: map[string]string{"color": "#3CBB3A"},
			create: []string{"mail", "settings", "folders", "create", "--name", "Projects", "--color", "#3CBB3A"},
		}},
	}, {
		what:   "filter",
		list:   []string{"mail", "settings", "filters", "list"},
		key:    "name",
		idKeys: []string{"id"},
		remove: []string{"mail", "settings", "filters", "delete"},
		// Disabled, and matching a word none of the fixture's mail carries. A free
		// account may hold one *active* filter, which the suite needs for itself,
		// and a filter that fired on the fixture's mail would file it out of the
		// inbox, which is where the panel looks.
		pins: []pin{{
			id:     "Archive newsletters",
			fields: map[string]string{"status": "0"},
			create: []string{"mail", "settings", "filters", "create", "--name", "Archive newsletters", "--disabled",
				"--sieve", `require ["fileinto"]; if header :contains "Subject" "newsletter" { fileinto "Archive"; }`},
		}},
	}, {
		what:   "contact",
		list:   []string{"contacts", "list"},
		key:    "name",
		idKeys: []string{"id"},
		remove: []string{"contacts", "delete"},
		pins: []pin{{
			id:     "Anna Berger",
			fields: map[string]string{"email": "anna@example.org"},
			create: []string{"contacts", "create", "--name", "Anna Berger", "--email", "anna@example.org",
				"--phone", "+43 1 234567", "--organization", "Berger & Co"},
		}},
	}, {
		what:   "vault",
		list:   []string{"pass", "vaults", "list"},
		key:    "name",
		idKeys: []string{"share_id"},
		remove: []string{"pass", "vaults", "delete"},
		pins: []pin{{
			id:     "Personal",
			create: []string{"pass", "vaults", "create", "--name", "Personal"},
		}},
	}, {
		what:   "pass item",
		list:   []string{"pass", "items", "list", "--vault", "Personal"},
		key:    "name",
		idKeys: []string{"item_id"},
		remove: []string{"pass", "items", "delete"},
		pins: []pin{{
			id:     "GitHub",
			fields: map[string]string{"type": "login"},
			create: []string{"pass", "items", "create", "--vault", "Personal", "--name", "GitHub",
				"--username", "roman", "--url", "github.com", "--password", "correct-horse-battery"},
		}, {
			id:     "Home Wi-Fi",
			fields: map[string]string{"type": "wifi"},
			create: []string{"pass", "items", "create", "--vault", "Personal", "--type", "wifi",
				"--name", "Home Wi-Fi", "--ssid", "Fritzbox", "--security", "WPA2", "--password", "hunter2hunter2"},
		}, {
			id:     "Door codes",
			fields: map[string]string{"type": "note"},
			create: []string{"pass", "items", "create", "--vault", "Personal", "--type", "note",
				"--name", "Door codes", "--note", "Front door: 1234"},
		}, {
			id:     "Travel card",
			fields: map[string]string{"type": "credit-card"},
			create: []string{"pass", "items", "create", "--vault", "Personal", "--type", "credit-card",
				"--name", "Travel card", "--holder", "Anna Berger", "--number", "4111111111111111",
				"--expiry", "2030-12", "--cvv", "123"},
		}},
	}, {
		what:   "drive",
		list:   []string{"drive", "items", "list", "/"},
		key:    "name",
		idKeys: []string{"link_id"},
		remove: []string{"drive", "items", "delete"},
		parent: "/",
		pins: []pin{{
			id:     "Documents",
			fields: map[string]string{"type": "folder"},
			create: []string{"drive", "folders", "create", "/Documents"},
		}},
	}, {
		what:   "drive",
		list:   []string{"drive", "items", "list", "/Documents"},
		key:    "name",
		idKeys: []string{"link_id"},
		remove: []string{"drive", "items", "delete"},
		parent: "/Documents",
		pins: []pin{{
			id:     "Trips",
			fields: map[string]string{"type": "folder"},
			create: []string{"drive", "folders", "create", "/Documents/Trips"},
		}, {
			id:     "packing-list.txt",
			fields: map[string]string{"type": "file"},
			create: []string{"drive", "items", "upload", filepath.Join(work, "packing-list.txt"), "/Documents"},
		}, {
			id:     "panorama.jpg",
			fields: map[string]string{"type": "file"},
			create: []string{"drive", "items", "upload", filepath.Join(work, "panorama.jpg"), "/Documents"},
		}},
	}, {
		what:   "drive",
		list:   []string{"drive", "items", "list", "/Documents/Trips"},
		key:    "name",
		idKeys: []string{"link_id"},
		remove: []string{"drive", "items", "delete"},
		parent: "/Documents/Trips",
		pins: []pin{{
			id:     "trail-map.txt",
			fields: map[string]string{"type": "file"},
			create: []string{"drive", "items", "upload", filepath.Join(work, "trail-map.txt"), "/Documents/Trips"},
		}},
	}, {
		what:   "event",
		list:   []string{"calendar", "events", "list", "--start", today(), "--end", inDays(30)},
		key:    "title",
		idKeys: []string{"calendar_id", "id"},
		remove: []string{"calendar", "events", "delete"},
		pins: []pin{{
			id: "Dentist",
			create: []string{"calendar", "events", "create", "--title", "Dentist",
				"--start", inDays(3) + "T10:00", "--duration", "1h",
				"--location", "Vienna", "--description", "Six-month check-up"},
		}, {
			id: "Standup",
			create: []string{"calendar", "events", "create", "--title", "Standup",
				"--start", inDays(3) + "T09:00", "--duration", "15m",
				"--rrule", "FREQ=WEEKLY;COUNT=5", "--remind", "15m"},
		}},
	}}
}

func today() string       { return time.Now().Format("2006-01-02") }
func inDays(n int) string { return time.Now().AddDate(0, 0, n).Format("2006-01-02") }
func inbox() []string     { return []string{"--folder", "inbox"} }
func attachArg(f string) []string {
	if f == "" {
		return nil
	}
	return []string{"--attach", f}
}

// mail brings the panel's three messages to the inbox.
//
// Mail is not a collection like the others: it is sent rather than created, it
// arrives a few seconds later, and what matters is which folder it lands in. A
// message the fixture wants but the inbox lacks is sent; one that is there is
// left where it is, read or not, because the suite marks messages as part of its
// own work.
func (r *report) mail(profile, address, work string) {
	for _, m := range panelMail {
		r.deliver(profile, address, work, fixture.Mail{Subject: m.subject, Body: m.body, Attach: m.attach})
	}
	// What the suite reads. Kept here rather than sent by the suite itself, so a
	// run spends its sending allowance on the send path it is testing.
	for _, m := range fixture.All() {
		r.deliver(profile, address, work, m)
	}
}

// deliver sends one message unless the inbox already holds it.
func (r *report) deliver(profile, address, work string, m fixture.Mail) {
	what := "mail: " + m.Subject
	found, err := rows(profile, append([]string{"mail", "messages", "search",
		"--subject", m.Subject, "--limit", "1"}, inbox()...)...)
	if err != nil {
		r.fail(what, err)
		return
	}
	if len(found) > 0 {
		return
	}
	send := []string{"mail", "messages", "send", "--to", address, "--subject", m.Subject, "--body", m.Body}
	if m.HTML {
		send = append(send, "--html")
	}
	if m.Attach != "" {
		send = append(send, "--attach", filepath.Join(work, m.Attach))
	}
	if m.Inline != "" {
		send = append(send, "--attach-inline", filepath.Join(work, m.Inline))
	}
	r.make(profile, what, send)
}

// calendarName is what the suite addresses the account's calendar by.
const calendarName = "Default"

// calendars is the collection a leftover test calendar is swept from.
//
// It pins nothing - the account arrives with its own calendar and the fixture
// renames that one - but it has to be swept like everything else, and more
// urgently: a free plan allows three, so a couple of calendars an interrupted run
// left behind is the difference between the suite working and every test that
// makes one failing on a limit it has nothing to do with.
var calendars = collection{
	what:   "calendar",
	list:   []string{"calendar", "settings", "calendars", "list"},
	key:    "name",
	idKeys: []string{"id"},
	remove: []string{"calendar", "settings", "calendars", "delete"},
}

// calendar makes the account's calendar answer to that name.
//
// An account arrives with one whose name varies - "My calendar" on these - and a
// free plan allows few enough calendars that adding one would take a slot the
// suite needs to create and delete its own. Renaming the one already there costs
// nothing.
func (r *report) calendar(profile string) {
	list, err := rows(profile, calendars.list...)
	if err != nil {
		r.fail("calendar: "+calendarName, err)
		return
	}
	// Swept first, so what is left to be named is the account's own calendar
	// rather than something a run left behind.
	r.sweep(profile, calendars, list)
	var kept []map[string]any
	for _, row := range list {
		if !strings.HasPrefix(str(row["name"]), testPrefix) {
			kept = append(kept, row)
		}
	}
	if _, ok := find(kept, "name", calendarName); ok {
		return
	}
	if len(kept) == 0 {
		r.fail("calendar: "+calendarName, fmt.Errorf("the account has no calendar to name"))
		return
	}
	r.remake(profile, "calendar: "+calendarName, []string{"calendar", "settings", "calendars",
		"update", str(kept[0]["id"]), "--name", calendarName})
}

// photoCount is how many photos the library has to hold.
const photoCount = 3

// photos tops the library up.
//
// A photo has no name - a row is a link ID, a capture time and two hashes - so
// the fixture pins how many there are rather than which they are. That is also
// what the tests want: they upload their own and diff the library around it,
// and a library with nothing in it is the one shape that tells them nothing.
func (r *report) photos(profile, work string) {
	list, err := rows(profile, "drive", "photos", "list")
	if err != nil {
		r.fail("photo", err)
		return
	}
	for i := len(list); i < photoCount; i++ {
		r.make(profile, fmt.Sprintf("photo: %d of %d", i+1, photoCount),
			[]string{"drive", "photos", "upload", filepath.Join(work, fmt.Sprintf("photo-%d.png", i+1))})
	}
}

// empty clears the three trashes.
//
// It runs last, so that what repair removed goes with it: a wrong label is
// deleted, a wrong file is trashed, and the panel's mail is re-sent over the
// top of trashed copies. Left alone all of that accumulates for as long as the
// accounts exist.
//
// Nothing here is recoverable afterwards, which is the point of a trash being
// empty, and is why it is the accounts kept for this that it runs against.
func (r *report) empty(profile string) {
	for _, e := range []struct {
		what string
		args []string
	}{
		{"drive trash", []string{"drive", "trash", "empty"}},
		{"pass trash", []string{"pass", "trash", "empty"}},
		{"mail trash", []string{"mail", "messages", "delete", "--folder", "trash", "--all"}},
	} {
		if _, err := run(profile, append([]string{"--yes"}, e.args...)...); err != nil {
			r.fail("empty "+e.what, err)
		}
	}
}

// stage makes the panel's three messages the only unread mail in the inbox.
//
// The panel opens on `mail messages list --unread`, and an inbox holds whatever
// Proton has sent it - share notifications the suite provoked, and Proton's own
// marketing. Neither can be kept out, so the recording is made deterministic
// from the other side: everything is marked read, the three are sent again, and
// they are the only unread mail there is.
func (r *report) stage(profile, address, work string) {
	if _, err := run(profile, append([]string{"--yes", "mail", "messages", "mark", "read", "--all"}, inbox()...)...); err != nil {
		r.fail("stage: mark the inbox read", err)
		return
	}
	for _, m := range panelMail {
		if _, err := run(profile, "--yes", "mail", "messages", "trash", "--subject", m.subject, "--limit", "20"); err != nil {
			r.fail("stage: clear "+m.subject, err)
			return
		}
	}
	// Sent oldest first, because an inbox lists newest first and the panel should
	// read in the order the fixture declares.
	for i := len(panelMail) - 1; i >= 0; i-- {
		m := panelMail[i]
		send := []string{"mail", "messages", "send", "--to", address, "--subject", m.subject, "--body", m.body}
		if m.attach != "" {
			send = append(send, attachArg(filepath.Join(work, m.attach))...)
		}
		if _, err := run(profile, send...); err != nil {
			r.fail("stage: send "+m.subject, err)
			return
		}
		r.note("+", profile, "mail: "+m.subject)
	}
	r.await(profile)
}

// await waits for the staged mail to arrive. Delivery runs a few seconds behind
// the send, and a recording follows.
func (r *report) await(profile string) {
	deadline := time.Now().Add(2 * time.Minute)
	for _, m := range panelMail {
		for {
			found, err := rows(profile, append([]string{"mail", "messages", "search",
				"--subject", m.subject, "--limit", "1"}, inbox()...)...)
			if err == nil && len(found) > 0 {
				break
			}
			if time.Now().After(deadline) {
				r.fail("stage: waiting for "+m.subject, fmt.Errorf("did not arrive"))
				break
			}
			time.Sleep(2 * time.Second)
		}
	}
}
