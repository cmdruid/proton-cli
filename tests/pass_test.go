package tests

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"
)

// ── vaults ──

func TestPassVaultsList(t *testing.T) {
	t.Parallel()
	stdout := runOK(t, "pass", "vaults", "list")
	assertContains(t, stdout, "ID")
}

// createVault makes a vault, waiting out the seconds Proton goes on counting one
// that has just been deleted.
//
// The free plan allows two and the fixture holds one, so every test that makes a
// vault takes the same spare slot. The lease hands it over the moment the delete
// returns, but the quota it is counted against catches up a few seconds later,
// and until it does the answer is that you cannot have another.
func createVault(t *testing.T, name string) string {
	t.Helper()
	var ref string
	waitFor(30*time.Second, 2*time.Second, func() bool {
		stdout, stderr, code := run(t, "pass", "vaults", "create", "--name", name)
		if code == 0 {
			ref = strings.TrimSpace(stdout)
			return true
		}
		if !strings.Contains(stderr, "cannot access more vaults") {
			t.Fatalf("creating a vault failed (exit %d): %s", code, stderr)
		}
		return false
	})
	if ref == "" {
		t.Fatal("the spare vault slot never came back")
	}
	return ref
}

func TestPassVaultsCRUD(t *testing.T) {
	t.Parallel()
	lease(t, vaultSlot)
	name := testID() + "-vault"
	shareID := createVault(t, name)
	if !looksLikeID(shareID) {
		t.Fatalf("expected bare share ID on stdout, got %q", shareID)
	}
	cleanupRun(t, fmt.Sprintf("Delete vault: proton pass vaults delete -- %s", shareID),
		"pass", "vaults", "delete", "--", shareID)

	list := runOK(t, "pass", "vaults", "list")
	assertContains(t, list, name)
}

// ── items: login ──

func TestPassItemsCRUDLogin(t *testing.T) {
	t.Parallel()
	name := testID() + "-login"
	url := "https://" + name + ".example.invalid/"

	stdout := runOK(t, "pass", "items", "create",
		"--type", "login",
		"--name", name,
		"--username", "tester",
		"--password", "s3cret!",
		"--url", url)
	itemID := strings.TrimSpace(stdout)
	if !looksLikePairRef(itemID) {
		t.Fatalf("expected SHARE_ID/ITEM_ID on stdout, got %q", stdout)
	}
	cleanupRun(t, fmt.Sprintf("Delete item: proton pass items delete %s", name),
		"pass", "items", "delete", name)

	// Get by URL REF
	got := runOK(t, "pass", "items", "get", name+".example.invalid")
	assertField(t, got, "Name:", name)
	assertField(t, got, "Username:", "tester")
	assertField(t, got, "Password:", "s3cret!")

	// Edit password
	runOK(t, "pass", "items", "update", name, "--password", "new-pass-v2")
	got2 := runOK(t, "pass", "items", "get", name)
	assertField(t, got2, "Password:", "new-pass-v2")
}

// ── items: note ──

func TestPassItemsCreateNote(t *testing.T) {
	t.Parallel()
	name := testID() + "-note"
	stdout := runOK(t, "pass", "items", "create",
		"--type", "note",
		"--name", name,
		"--note", "secret note content")
	id := strings.TrimSpace(stdout)
	if !looksLikePairRef(id) {
		t.Fatalf("expected SHARE_ID/ITEM_ID on stdout, got %q", stdout)
	}
	cleanupRun(t, fmt.Sprintf("Delete note: proton pass items delete %s", name),
		"pass", "items", "delete", name)

	got := runOK(t, "pass", "items", "get", name)
	assertField(t, got, "Type:", "note")
	assertField(t, got, "Note:", "secret note content")
}

// ── items: card (checks PIN rendering) ──

func TestPassItemsCreateCardShowsPIN(t *testing.T) {
	t.Parallel()
	name := testID() + "-card"
	stdout := runOK(t, "pass", "items", "create",
		"--type", "credit-card",
		"--name", name,
		"--holder", "Test Holder",
		"--number", "4111111111111111",
		"--expiry", "2029-01",
		"--cvv", "123",
		"--pin", "7890")
	id := strings.TrimSpace(stdout)
	if !looksLikeID(id) {
		t.Fatalf("expected bare ID on stdout, got %q", stdout)
	}
	cleanupRun(t, fmt.Sprintf("Delete card: proton pass items delete %s", name),
		"pass", "items", "delete", name)

	got := runOK(t, "pass", "items", "get", name)
	assertField(t, got, "Cardholder:", "Test Holder")
	assertField(t, got, "Number:", "4111111111111111")
	assertField(t, got, "Expiry:", "2029-01")
	assertField(t, got, "CVV:", "123")
	assertField(t, got, "PIN:", "7890")
}

// TestPassCreditCardTypeConsistent guards the D1 fix: the type word used at
// create time is the same word shown in output and accepted by the --type
// filter, so `create --type credit-card` then `trash --type credit-card`
// actually matches (the old card/credit_card split matched nothing).
func TestPassCreditCardTypeConsistent(t *testing.T) {
	t.Parallel()
	name := testID() + "-cc"
	ref := strings.TrimSpace(runOK(t, "pass", "items", "create", "--type", "credit-card",
		"--name", name, "--holder", "Roman", "--number", "4111111111111111", "--expiry", "2030-01"))
	cleanupRun(t, fmt.Sprintf("Delete card: proton pass items delete %s", name),
		"pass", "items", "delete", name)

	// Display/JSON type uses the same kebab spelling as the create flag.
	// --output json before -- so the flag parses and ref stays positional.
	var item map[string]interface{}
	if err := json.Unmarshal([]byte(runOK(t, "pass", "items", "get", "--output", "json", "--", ref)), &item); err != nil {
		t.Fatalf("parse item JSON: %v", err)
	}
	if got := item["type"]; got != "credit-card" {
		t.Errorf("type = %v, want credit-card", got)
	}
	// The --type filter word == the create word, so it matches the item.
	_, stderr := runOKStderr(t, "--dry-run", "pass", "items", "trash", "--type", "credit-card")
	if !strings.Contains(stderr, ref) {
		t.Errorf("trash --type credit-card should match the credit-card item %s; stderr:\n%s", ref, stderr)
	}
}

// ── items: trash / restore / delete ──

func TestPassItemsTrashRestoreDelete(t *testing.T) {
	t.Parallel()
	name := testID() + "-trash"
	stdout := runOK(t, "pass", "items", "create",
		"--type", "login", "--name", name,
		"--username", "u", "--password", "p")
	// Creating answers with SHARE_ID/ITEM_ID, which is the reference every item
	// verb takes - and the only way to reach a trashed item, since searching by
	// name does not find one.
	ref := strings.TrimSpace(stdout)
	cleanupRun(t, fmt.Sprintf("Delete item: proton pass items delete -- %s", ref),
		"pass", "items", "delete", "--", ref)

	runOK(t, "pass", "items", "trash", name)
	runOK(t, "pass", "trash", "restore", "--", ref)

	// It should be searchable again
	got := runOK(t, "pass", "items", "get", name)
	assertField(t, got, "Name:", name)
}

// ── items list with vault filter ──

func TestPassItemsListVaultFilter(t *testing.T) {
	t.Parallel()
	vaults := runJSONArray(t, "pass", "vaults", "list")
	if len(vaults) == 0 {
		t.Skip("no vaults")
	}
	firstName := vaults[0].(map[string]interface{})["name"].(string)
	runOK(t, "pass", "items", "list", "--vault", firstName)
}

// ── alias options (read-only) ──

func TestPassAliasOptions(t *testing.T) {
	t.Parallel()
	// Both kinds come back in one table, told apart by KIND rather than by two
	// headed sections.
	stdout := runOK(t, "pass", "aliases", "options")
	assertContains(t, stdout, "KIND")
	assertContains(t, stdout, "suffix")
	assertContains(t, stdout, "mailbox")
}

// An alias is an address Proton makes for you, so making one is its own request
// rather than another kind of item written locally.
func TestPassAliasesCreate(t *testing.T) {
	t.Parallel()
	name := testID() + "-alias"
	// The prefix becomes part of an email address, so it is short and plain, and
	// the item's name carries the suite's own prefix instead.
	prefix := fmt.Sprintf("pcli-%d", time.Now().UnixNano()%1_000_000_000)
	stdout, stderr := runOKStderr(t, "pass", "aliases", "create", "--prefix", prefix, "--name", name)
	ref := strings.TrimSpace(stdout)
	cleanupRun(t, fmt.Sprintf("Delete alias: proton pass items delete %s", name),
		"pass", "items", "delete", name)
	if !looksLikePairRef(ref) {
		t.Fatalf("expected SHARE_ID/ITEM_ID on stdout, got %q", ref)
	}

	// The address is what an alias is for, so creating one says which address it
	// made rather than the prefix it was asked for.
	assertContains(t, stderr, "@")
	said := addressIn(t, stderr)

	got := runJSON(t, "pass", "items", "get", "--", ref)
	if got["type"] != "alias" || got["name"] != name {
		t.Errorf("the item reads type %v name %v, want an alias called %s", got["type"], got["name"], name)
	}
	// The address Proton made from the prefix is the whole point of an alias, so
	// the item has to carry it. Proton appends a word of its own to the prefix.
	address, _ := got["alias"].(string)
	if !strings.HasPrefix(address, prefix) || !strings.Contains(address, "@") {
		t.Fatalf("alias address is %q, want an address built from %q", address, prefix)
	}
	if said != address {
		t.Errorf("creating said %q but the alias is %q", said, address)
	}
	assertContains(t, runOK(t, "pass", "items", "get", "--", ref), address)
	assertContains(t, runOK(t, "pass", "aliases", "list"), address)
}

// An address is half an answer: an alias is a route, so reading one says where
// its mail arrives, whether it is receiving at all, and what it has carried.
func TestPassAliasesGetShowsTheRoute(t *testing.T) {
	t.Parallel()
	ref, _ := makeAlias(t)

	got := runJSON(t, "pass", "items", "get", "--", ref)
	if got["alias_status"] != "enabled" {
		t.Errorf("a new alias reads status %v, want enabled", got["alias_status"])
	}
	boxes, _ := got["alias_mailboxes"].([]interface{})
	if len(boxes) == 0 {
		t.Fatalf("the alias forwards nowhere: %v", got)
	}
	if mailbox, _ := boxes[0].(string); !strings.Contains(mailbox, "@") {
		t.Errorf("it forwards to %q, want an address", mailbox)
	}
	if _, ok := got["alias_activity"].(map[string]interface{}); !ok {
		t.Errorf("no activity on the alias: %v", got)
	}
	assertField(t, runOK(t, "pass", "items", "get", "--", ref), "Forwards To:", boxes[0].(string))
}

// An alias that starts attracting spam is switched off, not deleted: deleting it
// burns the address, and nothing brings it back.
func TestPassAliasesDisableAndEnable(t *testing.T) {
	t.Parallel()
	ref, address := makeAlias(t)

	_, stderr := runOKStderr(t, "pass", "aliases", "disable", "--", ref)
	assertContains(t, stderr, "Disabled alias")
	if got := runJSON(t, "pass", "items", "get", "--", ref); got["alias_status"] != "disabled" {
		t.Errorf("after disabling, the alias reads %v", got["alias_status"])
	}
	// The list knows without asking after each address, so it says so too.
	listed := runOK(t, "pass", "aliases", "list")
	assertContains(t, listed, "disabled")
	assertContains(t, listed, address)

	runOK(t, "pass", "aliases", "enable", "--", ref)
	if got := runJSON(t, "pass", "items", "get", "--", ref); got["alias_status"] != "enabled" {
		t.Errorf("after enabling, the alias reads %v", got["alias_status"])
	}
}

// Where an alias forwards and what it sends as are fields of the item, so they
// are changed by the same command that changes every other field.
func TestPassItemsUpdateAliasRoute(t *testing.T) {
	t.Parallel()
	ref, _ := makeAlias(t)
	mailbox := runJSON(t, "pass", "items", "get", "--", ref)["alias_mailboxes"].([]interface{})[0].(string)
	sender := "Jane " + testID()

	runOK(t, "pass", "items", "update", "--mailbox", mailbox, "--display-name", sender, "--", ref)

	got := runJSON(t, "pass", "items", "get", "--", ref)
	boxes, _ := got["alias_mailboxes"].([]interface{})
	if len(boxes) != 1 || boxes[0] != mailbox {
		t.Errorf("the alias forwards to %v, want %q", boxes, mailbox)
	}
	if got["alias_display_name"] != sender {
		t.Errorf("the alias sends as %v, want %q", got["alias_display_name"], sender)
	}
}

// makeAlias creates an alias and hands back its reference and its address.
func makeAlias(t *testing.T) (ref, address string) {
	t.Helper()
	name := testID() + "-alias"
	prefix := fmt.Sprintf("pcli-%d", time.Now().UnixNano()%1_000_000_000)
	stdout, stderr := runOKStderr(t, "pass", "aliases", "create", "--prefix", prefix, "--name", name)
	ref = strings.TrimSpace(stdout)
	cleanupRun(t, fmt.Sprintf("Delete alias: proton pass items delete %s", name),
		"pass", "items", "delete", name)
	return ref, addressIn(t, stderr)
}

// addressIn picks the email address out of a confirmation line.
func addressIn(t *testing.T, line string) string {
	t.Helper()
	for _, word := range strings.Fields(strings.TrimSuffix(strings.TrimSpace(line), ".")) {
		if strings.Contains(word, "@") {
			return strings.TrimSuffix(word, ".")
		}
	}
	t.Fatalf("no address in %q", line)
	return ""
}

// ── batch filters (all dry-run) ──

func TestPassBatchTrashDryRunByType(t *testing.T) {
	t.Parallel()
	_, stderr, code := run(t, "--dry-run", "pass", "items", "trash", "--type", "note")
	if code != 0 {
		t.Fatalf("dry-run should succeed, got exit %d: %s", code, stderr)
	}
	assertContains(t, stderr, "Dry run")
}

func TestPassBatchTrashDryRunOlderThanYear(t *testing.T) {
	t.Parallel()
	_, stderr, code := run(t, "--dry-run", "pass", "items", "trash",
		"--older-than", "1y", "--type", "login")
	if code != 0 {
		t.Fatalf("dry-run should succeed, got exit %d: %s", code, stderr)
	}
	// Either a "would trash" line or nothing to trash; at minimum doesn't crash
	_ = stderr
}

func TestPassBatchTrashDurationUnitMonths(t *testing.T) {
	t.Parallel()
	// "6mo" must parse without error.
	_, _, code := run(t, "--dry-run", "pass", "items", "trash",
		"--older-than", "6mo", "--type", "login")
	if code != 0 {
		t.Errorf("--older-than 6mo should parse, got exit %d", code)
	}
}

func TestPassItemTypesAndFields(t *testing.T) {
	t.Parallel()
	// Identity with core fields plus custom text/hidden fields.
	idName := testID() + "-identity"
	idRef := strings.TrimSpace(runOK(t, "pass", "items", "create", "--type", "identity",
		"--name", idName, "--full-name", "Jane Roe", "--email", "jane@example.com",
		"--organization", "Acme", "--field", "Note=hello-field", "--hidden", "PIN=4321"))
	cleanupRun(t, fmt.Sprintf("Delete pass item: proton pass items delete %s", idRef),
		"pass", "items", "delete", "--", idRef)
	gotID := runOK(t, "pass", "items", "get", "--", idRef)
	assertContains(t, gotID, "Jane Roe")
	assertContains(t, gotID, "Acme")
	assertContains(t, gotID, "hello-field")

	// Wi-Fi.
	wifiRef := strings.TrimSpace(runOK(t, "pass", "items", "create", "--type", "wifi",
		"--name", testID()+"-wifi", "--ssid", "MyTestNet", "--password", "pw", "--security", "WPA2"))
	cleanupRun(t, fmt.Sprintf("Delete pass item: proton pass items delete %s", wifiRef),
		"pass", "items", "delete", "--", wifiRef)
	assertContains(t, runOK(t, "pass", "items", "get", "--", wifiRef), "MyTestNet")

	// SSH key.
	sshRef := strings.TrimSpace(runOK(t, "pass", "items", "create", "--type", "ssh-key",
		"--name", testID()+"-ssh", "--public-key", "ssh-ed25519 AAAATESTKEY", "--private-key", "PRIVATE-TEST"))
	cleanupRun(t, fmt.Sprintf("Delete pass item: proton pass items delete %s", sshRef),
		"pass", "items", "delete", "--", sshRef)
	assertContains(t, runOK(t, "pass", "items", "get", "--", sshRef), "ssh-ed25519 AAAATESTKEY")
}

func TestPassVaultRename(t *testing.T) {
	t.Parallel()
	lease(t, vaultSlot)
	name := testID() + "-vault"
	sid := createVault(t, name)
	cleanupRun(t, fmt.Sprintf("Delete vault: proton pass vaults delete %s", sid),
		"pass", "vaults", "delete", "--", sid)

	newName := name + "-renamed"
	runOK(t, "pass", "vaults", "update", "--name", newName, sid)
	assertContains(t, runOK(t, "pass", "vaults", "list"), newName)
}

func TestPassLoginTOTPRoundTrips(t *testing.T) {
	t.Parallel()
	name := testID() + "-totp"
	secret := "JBSWY3DPEHPK3PXP"
	ref := strings.TrimSpace(runOK(t, "pass", "items", "create", "--type", "login",
		"--name", name, "--username", "me@example.com",
		"--totp-uri", "otpauth://totp/Example:me?secret="+secret+"&issuer=Example"))
	cleanupRun(t, fmt.Sprintf("Delete pass item: proton pass items delete %s", ref),
		"pass", "items", "delete", "--", ref)

	assertContains(t, runOK(t, "pass", "items", "get", "--", ref), secret)
}
