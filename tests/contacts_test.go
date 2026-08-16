package tests

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	gopenpgp "github.com/ProtonMail/gopenpgp/v2/crypto"
)

// writeGeneratedPubKey generates a throwaway key pair and writes its armored
// public key to a temp .asc file, returning the path.
func writeGeneratedPubKey(t *testing.T) string {
	t.Helper()
	key, err := gopenpgp.GenerateKey("pin-test", "pin@example.invalid", "x25519", 0)
	if err != nil {
		t.Fatal(err)
	}
	pub, err := key.GetArmoredPublicKey()
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "key.asc")
	if err := os.WriteFile(path, []byte(pub), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

// signedCardData returns the cleartext Data of a contact's signed (Type-2)
// card, fetched raw via the API. Pinned KEY properties live here.
func signedCardData(t *testing.T, contactID string) string {
	t.Helper()
	data := runJSON(t, "api", "GET", "/contacts/v4/contacts/"+contactID)
	contact, _ := data["Contact"].(map[string]interface{})
	cards, _ := contact["Cards"].([]interface{})
	for _, c := range cards {
		m, _ := c.(map[string]interface{})
		if tp, _ := m["Type"].(float64); int(tp) == 2 {
			s, _ := m["Data"].(string)
			return s
		}
	}
	return ""
}

func TestContactsPinUnpinKey(t *testing.T) {
	t.Parallel()
	email := "pin-" + testID() + "@example.invalid"
	id := strings.TrimSpace(runOK(t, "contacts", "create", "--name", testID()+"-pin", "--email", email))
	cleanupRun(t, fmt.Sprintf("Delete contact: proton contacts delete %s", id),
		"contacts", "delete", "--", id)

	runOK(t, "contacts", "keys", "pin", "--key", writeGeneratedPubKey(t), "--email", email, id)
	if !strings.Contains(signedCardData(t, id), "KEY;") {
		t.Error("expected a pinned KEY property in the signed card after pin-key")
	}

	runOK(t, "contacts", "keys", "unpin", "--email", email, id)
	if strings.Contains(signedCardData(t, id), "KEY;") {
		t.Error("KEY property should be gone after unpin-key")
	}
}

func TestContactsUpdatePreservesPinnedKey(t *testing.T) {
	t.Parallel()
	email := "pin-" + testID() + "@example.invalid"
	id := strings.TrimSpace(runOK(t, "contacts", "create", "--name", testID()+"-pin", "--email", email))
	cleanupRun(t, fmt.Sprintf("Delete contact: proton contacts delete %s", id),
		"contacts", "delete", "--", id)

	runOK(t, "contacts", "keys", "pin", "--key", writeGeneratedPubKey(t), "--email", email, id)
	if !strings.Contains(signedCardData(t, id), "KEY;") {
		t.Fatal("setup: pinned key missing after pin-key")
	}

	// An unrelated field update must not drop the pinned key.
	runOK(t, "contacts", "update", "--job-title", "Boss", id)
	if !strings.Contains(signedCardData(t, id), "KEY;") {
		t.Error("contacts update dropped the pinned key")
	}
}

// TestMailSendToPinnedContactStillDelivers pins the second account's real public key on a
// contact and sends to it: a matching pin must not break E2EE delivery, and
// the second account must still decrypt the body with a verified signature.
func TestMailSendToPinnedContactStillDelivers(t *testing.T) {
	t.Parallel()
	data := runJSON(t, "api", "GET", "/core/v4/keys/all", "--query", "Email="+secondaryEmail(), "--query", "InternalOnly=0")
	addr, _ := data["Address"].(map[string]interface{})
	ks, _ := addr["Keys"].([]interface{})
	if len(ks) == 0 {
		t.Skip("no public key published for the second account")
	}
	pub, _ := ks[0].(map[string]interface{})["PublicKey"].(string)
	keyPath := filepath.Join(t.TempDir(), "secondary.asc")
	if err := os.WriteFile(keyPath, []byte(pub), 0o644); err != nil {
		t.Fatal(err)
	}

	id := strings.TrimSpace(runOK(t, "contacts", "create", "--name", testID()+"-altpin", "--email", secondaryEmail()))
	cleanupRun(t, fmt.Sprintf("Delete contact: proton contacts delete %s", id),
		"contacts", "delete", "--", id)
	runOK(t, "contacts", "keys", "pin", "--key", keyPath, "--email", secondaryEmail(), id)

	subject := testID() + "-pinned-send"
	body := "pinned-key body for " + subject
	runOK(t, "mail", "messages", "send", "--to", secondaryEmail(), "--subject", subject, "--body", body)
	if sentID := findMessage(t, "sent", subject); sentID != "" {
		cleanupRun(t, "Delete sent mail: proton mail messages delete "+sentID,
			"mail", "messages", "delete", sentID)
	}

	var recvID string
	waitFor(45*time.Second, 3*time.Second, func() bool {
		recvID = secondaryMailContaining(t, selfEmail(), body)
		return recvID != ""
	})
	if recvID == "" {
		t.Fatal("the second account did not receive the pinned-key mail")
	}
	cleanupRunSecondary(t, "Delete received mail (secondary): proton --profile secondary mail messages delete "+recvID,
		"mail", "messages", "delete", recvID)

	read := runOKSecondary(t, "mail", "messages", "get", recvID)
	assertContains(t, read, body)
	assertField(t, read, "Signature:", "verified")
}

// TestMailSendPinnedMismatchRefused pins a wrong key on a contact for a Proton
// recipient: the send must refuse (the recipient's primary key isn't among the
// pinned keys) and must not leak the draft it created. The second assertion is
// a regression guard for the send-abort cleanup, which used the wrong HTTP
// method and silently leaked drafts on any aborted send.
func TestMailSendPinnedMismatchRefused(t *testing.T) {
	t.Parallel()
	id := strings.TrimSpace(runOK(t, "contacts", "create", "--name", testID()+"-mismatch", "--email", secondaryEmail()))
	cleanupRun(t, fmt.Sprintf("Delete contact: proton contacts delete %s", id),
		"contacts", "delete", "--", id)
	// A freshly generated key is a valid PGP key but not the second account's.
	runOK(t, "contacts", "keys", "pin", "--key", writeGeneratedPubKey(t), "--email", secondaryEmail(), id)

	subject := testID() + "-mismatch"
	_, stderr, code := run(t, "mail", "messages", "send", "--to", secondaryEmail(), "--subject", subject, "--body", "nope")
	if code != 1 {
		t.Errorf("expected exit 1 on a pinned-key mismatch, got %d (stderr: %s)", code, stderr)
	}
	if !strings.Contains(stderr, "do not match") {
		t.Errorf("expected a primary-not-pinned message, got: %s", stderr)
	}

	// The aborted send must not leave its draft behind.
	if leaked := messageIDInFolder("drafts", subject); leaked != "" {
		cleanupRun(t, "Delete leaked draft: proton mail messages delete "+leaked,
			"mail", "messages", "delete", leaked)
		t.Errorf("aborted send leaked a draft into Drafts: %s", leaked)
	}
}

func TestContactsList(t *testing.T) {
	t.Parallel()
	stdout := runOK(t, "contacts", "list")
	assertContains(t, stdout, "NAME")
}

func TestContactsCRUD(t *testing.T) {
	t.Parallel()
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
	cleanupRun(t, fmt.Sprintf("Delete contact: proton contacts delete -- %s", id),
		"contacts", "delete", "--", id)

	// Get by explicit ID
	got := runOK(t, "contacts", "get", "--", id)
	assertField(t, got, "Name:", name)
	assertField(t, got, "Email:", email)
	// Signature: a contact we just created is signed with our own user key.
	assertField(t, got, "Signature:", "verified")
	assertField(t, got, "Phone:", "+1234567890")

	// Update phone
	runOK(t, "contacts", "update", "--phone", "+9999999999", "--", id)
	got2 := runOK(t, "contacts", "get", "--", id)
	assertField(t, got2, "Phone:", "+9999999999")
	// name/email unchanged
	assertField(t, got2, "Name:", name)
}

func TestContactsGetByNameRef(t *testing.T) {
	t.Parallel()
	name := testID() + "-refname"
	stdout := runOK(t, "contacts", "create", "--name", name, "--email", "t@x.invalid")
	id := strings.TrimSpace(stdout)
	cleanupRun(t, fmt.Sprintf("Delete contact: proton contacts delete -- %s", id),
		"contacts", "delete", "--", id)

	got := runOK(t, "contacts", "get", name)
	assertField(t, got, "Name:", name)
}

func TestContactsGetByEmailRef(t *testing.T) {
	t.Parallel()
	name := testID() + "-refmail"
	email := "t+" + name + "@x.invalid"
	stdout := runOK(t, "contacts", "create", "--name", name, "--email", email)
	id := strings.TrimSpace(stdout)
	cleanupRun(t, fmt.Sprintf("Delete contact: proton contacts delete -- %s", id),
		"contacts", "delete", "--", id)

	got := runOK(t, "contacts", "get", email)
	assertField(t, got, "Email:", email)
}

func TestContactsDeleteByRef(t *testing.T) {
	t.Parallel()
	name := testID() + "-refdel"
	runOK(t, "contacts", "create", "--name", name, "--email", "t@x.invalid")

	runOK(t, "contacts", "delete", name)
	_, _, code := run(t, "contacts", "get", name)
	if code != 3 {
		t.Errorf("expected exit 3 after delete, got %d", code)
	}
}

func TestContactsNotFound(t *testing.T) {
	t.Parallel()
	_, _, code := run(t, "contacts", "get", "no-such-contact-"+testID())
	if code != 3 {
		t.Errorf("expected exit 3 for unknown contact, got %d", code)
	}
}

func TestContactsAmbiguous(t *testing.T) {
	t.Parallel()
	prefix := testID() + "-ambig"
	for i := 0; i < 2; i++ {
		stdout := runOK(t, "contacts", "create",
			"--name", fmt.Sprintf("%s-%d", prefix, i),
			"--email", fmt.Sprintf("a%d@x.invalid", i))
		id := strings.TrimSpace(stdout)
		cleanupRun(t, fmt.Sprintf("Delete contact: proton contacts delete -- %s", id),
			"contacts", "delete", "--", id)
	}
	_, _, code := run(t, "contacts", "get", prefix)
	if code != 4 {
		t.Errorf("expected exit 4 for ambiguous match, got %d", code)
	}
}

func TestContactsMultiValue(t *testing.T) {
	t.Parallel()
	name := testID() + "-mv"
	e1 := testID() + "-1@example.com"
	e2 := testID() + "-2@example.com"
	cid := strings.TrimSpace(runOK(t, "contacts", "create", "--name", name,
		"--email", e1, "--email", e2, "--phone", "+1234567890",
		"--job-title", "CTO", "--birthday", "1990-01-31", "--address", "Vienna", "--website", "https://x.example"))
	cleanupRun(t, fmt.Sprintf("Delete contact: proton contacts delete %s", cid),
		"contacts", "delete", "--", cid)

	got := runOK(t, "contacts", "get", "--", cid)
	assertContains(t, got, e1)
	assertContains(t, got, e2)
	assertContains(t, got, "CTO")
	assertContains(t, got, "Vienna")
}

func TestContactsGroups(t *testing.T) {
	t.Parallel()
	gname := testID() + "-group"
	stdout, stderr, code := run(t, "contacts", "groups", "create", "--name", gname, "--color", "#8080FF")
	if code != 0 {
		// Proton answers 2027 when the account's plan does not include contact
		// groups. Matched on the code rather than the sentence, which is Proton's
		// to reword.
		if strings.Contains(stderr, "2027") {
			t.Skip("contact groups need a paid plan and this account does not have one")
		}
		t.Fatalf("groups create failed (exit %d): %s", code, truncateOutput(stderr))
	}
	gid := strings.TrimSpace(stdout)
	cleanupRun(t, fmt.Sprintf("Delete group: proton contacts groups delete %s", gid),
		"contacts", "groups", "delete", "--", gid)

	cname := testID() + "-gc"
	cid := strings.TrimSpace(runOK(t, "contacts", "create", "--name", cname, "--email", testID()+"@example.com"))
	cleanupRun(t, fmt.Sprintf("Delete contact: proton contacts delete %s", cid),
		"contacts", "delete", "--", cid)

	runOK(t, "contacts", "groups", "add", gid, cid)
	assertContains(t, runOK(t, "contacts", "groups", "list"), gname)
	runOK(t, "contacts", "groups", "remove", gid, cid)
}
