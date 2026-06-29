package tests

import (
	"fmt"
	"strings"
	"testing"
)

func TestContactsList(t *testing.T) {
	skipIfNoCredentials(t)
	stdout := runOK(t, "contacts", "list")
	assertContains(t, stdout, "NAME")
}

func TestContactsCRUD(t *testing.T) {
	skipIfNoCredentials(t)
	name := testID() + "-contact"
	email := "test+" + name + "@example.invalid"

	stdout := runOK(t, "contacts", "create",
		"--name", name,
		"--email", email,
		"--phone", "+1234567890")
	id := strings.TrimSpace(stdout)
	if !looksLikeID(id) {
		t.Fatalf("expected bare ID on stdout, got %q", stdout)
	}
	cleanupRun(t, fmt.Sprintf("Delete contact: proton-cli contacts delete -- %s", id),
		"contacts", "delete", "--", id)

	// Get by explicit ID
	got := runOK(t, "contacts", "get", "--", id)
	assertField(t, got, "Name:", name)
	assertField(t, got, "Email:", email)
	// Signature: a contact we just created is signed with our own user key.
	assertField(t, got, "Sig:", "verified")
	assertField(t, got, "Phone:", "+1234567890")

	// Update phone
	runOK(t, "contacts", "update", "--phone", "+9999999999", "--", id)
	got2 := runOK(t, "contacts", "get", "--", id)
	assertField(t, got2, "Phone:", "+9999999999")
	// name/email unchanged
	assertField(t, got2, "Name:", name)
}

func TestContactsGetByNameRef(t *testing.T) {
	skipIfNoCredentials(t)
	name := testID() + "-refname"
	stdout := runOK(t, "contacts", "create", "--name", name, "--email", "t@x.invalid")
	id := strings.TrimSpace(stdout)
	cleanupRun(t, fmt.Sprintf("Delete contact: proton-cli contacts delete -- %s", id),
		"contacts", "delete", "--", id)

	got := runOK(t, "contacts", "get", name)
	assertField(t, got, "Name:", name)
}

func TestContactsGetByEmailRef(t *testing.T) {
	skipIfNoCredentials(t)
	name := testID() + "-refmail"
	email := "t+" + name + "@x.invalid"
	stdout := runOK(t, "contacts", "create", "--name", name, "--email", email)
	id := strings.TrimSpace(stdout)
	cleanupRun(t, fmt.Sprintf("Delete contact: proton-cli contacts delete -- %s", id),
		"contacts", "delete", "--", id)

	got := runOK(t, "contacts", "get", email)
	assertField(t, got, "Email:", email)
}

func TestContactsDeleteByRef(t *testing.T) {
	skipIfNoCredentials(t)
	name := testID() + "-refdel"
	runOK(t, "contacts", "create", "--name", name, "--email", "t@x.invalid")

	runOK(t, "contacts", "delete", name)
	_, _, code := run(t, "contacts", "get", name)
	if code != 3 {
		t.Errorf("expected exit 3 after delete, got %d", code)
	}
}

func TestContactsNotFound(t *testing.T) {
	skipIfNoCredentials(t)
	_, _, code := run(t, "contacts", "get", "no-such-contact-"+testID())
	if code != 3 {
		t.Errorf("expected exit 3 for unknown contact, got %d", code)
	}
}

func TestContactsAmbiguous(t *testing.T) {
	skipIfNoCredentials(t)
	prefix := testID() + "-ambig"
	for i := 0; i < 2; i++ {
		stdout := runOK(t, "contacts", "create",
			"--name", fmt.Sprintf("%s-%d", prefix, i),
			"--email", fmt.Sprintf("a%d@x.invalid", i))
		id := strings.TrimSpace(stdout)
		cleanupRun(t, fmt.Sprintf("Delete contact: proton-cli contacts delete -- %s", id),
			"contacts", "delete", "--", id)
	}
	_, _, code := run(t, "contacts", "get", prefix)
	if code != 4 {
		t.Errorf("expected exit 4 for ambiguous match, got %d", code)
	}
}

func TestContactsMultiValue(t *testing.T) {
	skipIfNoCredentials(t)

	name := testID() + "-mv"
	e1 := testID() + "-1@example.com"
	e2 := testID() + "-2@example.com"
	cid := strings.TrimSpace(runOK(t, "contacts", "create", "--name", name,
		"--email", e1, "--email", e2, "--phone", "+1234567890",
		"--title", "CTO", "--birthday", "1990-01-31", "--address", "Vienna", "--url", "https://x.example"))
	cleanupRun(t, fmt.Sprintf("Delete contact: proton-cli contacts delete %s", cid),
		"contacts", "delete", "--", cid)

	got := runOK(t, "contacts", "get", "--", cid)
	assertContains(t, got, e1)
	assertContains(t, got, e2)
	assertContains(t, got, "CTO")
	assertContains(t, got, "Vienna")
}

func TestContactsGroups(t *testing.T) {
	skipIfNoCredentials(t)

	gname := testID() + "-group"
	gid := strings.TrimSpace(runOK(t, "contacts", "groups", "create", "--name", gname, "--color", "#8080FF"))
	cleanupRun(t, fmt.Sprintf("Delete group: proton-cli contacts groups delete %s", gid),
		"contacts", "groups", "delete", "--", gid)

	cname := testID() + "-gc"
	cid := strings.TrimSpace(runOK(t, "contacts", "create", "--name", cname, "--email", testID()+"@example.com"))
	cleanupRun(t, fmt.Sprintf("Delete contact: proton-cli contacts delete %s", cid),
		"contacts", "delete", "--", cid)

	runOK(t, "contacts", "groups", "add", gid, cid)
	assertContains(t, runOK(t, "contacts", "groups", "list"), gname)
	runOK(t, "contacts", "groups", "remove", gid, cid)
}
