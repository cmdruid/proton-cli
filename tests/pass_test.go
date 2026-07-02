package tests

import (
	"fmt"
	"strings"
	"testing"
)

// ── vaults ──

func TestPassVaultsList(t *testing.T) {
	stdout := runOK(t, "pass", "vaults", "list")
	assertContains(t, stdout, "SHARE_ID")
}

func TestPassVaultsCRUD(t *testing.T) {
	name := testID() + "-vault"
	stdout := runOK(t, "pass", "vaults", "create", "--name", name)
	shareID := strings.TrimSpace(stdout)
	if !looksLikeID(shareID) {
		t.Fatalf("expected bare share ID on stdout, got %q", stdout)
	}
	cleanupRun(t, fmt.Sprintf("Delete vault: proton-cli pass vaults delete -- %s", shareID),
		"pass", "vaults", "delete", "--", shareID)

	list := runOK(t, "pass", "vaults", "list")
	assertContains(t, list, name)
}

// ── items: login ──

func TestPassItemsCRUDLogin(t *testing.T) {
	name := testID() + "-login"
	url := "https://" + name + ".example.invalid/"

	stdout := runOK(t, "pass", "items", "create",
		"--type", "login",
		"--name", name,
		"--username", "tester",
		"--password", "s3cret!",
		"--url", url)
	itemID := strings.TrimSpace(stdout)
	if !looksLikeID(itemID) {
		t.Fatalf("expected bare item ID on stdout, got %q", stdout)
	}
	cleanupRun(t, fmt.Sprintf("Delete item: proton-cli pass items delete %s", name),
		"pass", "items", "delete", name)

	// Get by URL REF
	got := runOK(t, "pass", "items", "get", name+".example.invalid")
	assertField(t, got, "Name:", name)
	assertField(t, got, "Username:", "tester")
	assertField(t, got, "Password:", "s3cret!")

	// Edit password
	runOK(t, "pass", "items", "edit", name, "--password", "new-pass-v2")
	got2 := runOK(t, "pass", "items", "get", name)
	assertField(t, got2, "Password:", "new-pass-v2")
}

// ── items: note ──

func TestPassItemsCreateNote(t *testing.T) {
	name := testID() + "-note"
	stdout := runOK(t, "pass", "items", "create",
		"--type", "note",
		"--name", name,
		"--note", "secret note content")
	id := strings.TrimSpace(stdout)
	if !looksLikeID(id) {
		t.Fatalf("expected bare ID on stdout, got %q", stdout)
	}
	cleanupRun(t, fmt.Sprintf("Delete note: proton-cli pass items delete %s", name),
		"pass", "items", "delete", name)

	got := runOK(t, "pass", "items", "get", name)
	assertField(t, got, "Type:", "note")
	assertField(t, got, "Note:", "secret note content")
}

// ── items: card (checks PIN rendering) ──

func TestPassItemsCreateCardShowsPIN(t *testing.T) {
	name := testID() + "-card"
	stdout := runOK(t, "pass", "items", "create",
		"--type", "card",
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
	cleanupRun(t, fmt.Sprintf("Delete card: proton-cli pass items delete %s", name),
		"pass", "items", "delete", name)

	got := runOK(t, "pass", "items", "get", name)
	assertField(t, got, "Holder:", "Test Holder")
	assertField(t, got, "Number:", "4111111111111111")
	assertField(t, got, "Expiry:", "2029-01")
	assertField(t, got, "CVV:", "123")
	assertField(t, got, "PIN:", "7890")
}

// ── items: trash / restore / delete ──

func TestPassItemsTrashRestoreDelete(t *testing.T) {
	name := testID() + "-trash"
	stdout := runOK(t, "pass", "items", "create",
		"--type", "login", "--name", name,
		"--username", "u", "--password", "p")
	itemID := strings.TrimSpace(stdout)

	// Need share ID for restore (trashed items don't appear in search)
	vaults := runJSONArray(t, "pass", "vaults", "list")
	shareID := vaults[0].(map[string]interface{})["share_id"].(string)

	// Register a best-effort cleanup (permanent delete by IDs)
	cleanupRun(t, fmt.Sprintf("Delete item: proton-cli pass items delete -- %s %s", shareID, itemID),
		"pass", "items", "delete", "--", shareID, itemID)

	runOK(t, "pass", "items", "trash", name)
	runOK(t, "pass", "items", "restore", "--", shareID, itemID)

	// It should be searchable again
	got := runOK(t, "pass", "items", "get", name)
	assertField(t, got, "Name:", name)
}

// ── items list with vault filter ──

func TestPassItemsListVaultFilter(t *testing.T) {
	vaults := runJSONArray(t, "pass", "vaults", "list")
	if len(vaults) == 0 {
		t.Skip("no vaults")
	}
	firstName := vaults[0].(map[string]interface{})["name"].(string)
	runOK(t, "pass", "items", "list", "--vault", firstName)
}

// ── alias options (read-only) ──

func TestPassAliasOptions(t *testing.T) {
	stdout := runOK(t, "pass", "alias", "options")
	assertContains(t, stdout, "Suffixes")
	assertContains(t, stdout, "Mailboxes")
}

// ── batch filters (all dry-run) ──

func TestPassBatchTrashDryRunByType(t *testing.T) {
	_, stderr, code := run(t, "--dry-run", "pass", "items", "trash", "--type", "note")
	if code != 0 {
		t.Fatalf("dry-run should succeed, got exit %d: %s", code, stderr)
	}
	assertContains(t, stderr, "dry-run")
}

func TestPassBatchTrashDryRunOlderThanYear(t *testing.T) {
	_, stderr, code := run(t, "--dry-run", "pass", "items", "trash",
		"--older-than", "1y", "--type", "login")
	if code != 0 {
		t.Fatalf("dry-run should succeed, got exit %d: %s", code, stderr)
	}
	// Either a "would trash" line or nothing to trash; at minimum doesn't crash
	_ = stderr
}

func TestPassBatchTrashDurationUnitMonths(t *testing.T) {
	// "6mo" must parse without error.
	_, _, code := run(t, "--dry-run", "pass", "items", "trash",
		"--older-than", "6mo", "--type", "login")
	if code != 0 {
		t.Errorf("--older-than 6mo should parse, got exit %d", code)
	}
}

func TestPassBatchTrashRequiresInput(t *testing.T) {
	_, stderr, code := run(t, "pass", "items", "trash")
	if code == 0 {
		t.Error("expected error when no REF and no filter given")
	}
	assertContains(t, stderr, "no items selected")
}

func TestPassItemTypesAndFields(t *testing.T) {
	// Identity with core fields plus custom text/hidden fields.
	idName := testID() + "-identity"
	idRef := strings.TrimSpace(runOK(t, "pass", "items", "create", "--type", "identity",
		"--name", idName, "--full-name", "Jane Roe", "--email", "jane@example.com",
		"--organization", "Acme", "--field", "Note=hello-field", "--hidden", "PIN=4321"))
	cleanupRun(t, fmt.Sprintf("Delete pass item: proton-cli pass items delete %s", idRef),
		"pass", "items", "delete", "--", idRef)
	gotID := runOK(t, "pass", "items", "get", "--", idRef)
	assertContains(t, gotID, "Jane Roe")
	assertContains(t, gotID, "Acme")
	assertContains(t, gotID, "hello-field")

	// Wi-Fi.
	wifiRef := strings.TrimSpace(runOK(t, "pass", "items", "create", "--type", "wifi",
		"--name", testID()+"-wifi", "--ssid", "MyTestNet", "--password", "pw", "--security", "WPA2"))
	cleanupRun(t, fmt.Sprintf("Delete pass item: proton-cli pass items delete %s", wifiRef),
		"pass", "items", "delete", "--", wifiRef)
	assertContains(t, runOK(t, "pass", "items", "get", "--", wifiRef), "MyTestNet")

	// SSH key.
	sshRef := strings.TrimSpace(runOK(t, "pass", "items", "create", "--type", "ssh_key",
		"--name", testID()+"-ssh", "--public-key", "ssh-ed25519 AAAATESTKEY", "--private-key", "PRIVATE-TEST"))
	cleanupRun(t, fmt.Sprintf("Delete pass item: proton-cli pass items delete %s", sshRef),
		"pass", "items", "delete", "--", sshRef)
	assertContains(t, runOK(t, "pass", "items", "get", "--", sshRef), "ssh-ed25519 AAAATESTKEY")
}

func TestPassVaultRename(t *testing.T) {
	name := testID() + "-vault"
	sid := strings.TrimSpace(runOK(t, "pass", "vaults", "create", "--name", name))
	cleanupRun(t, fmt.Sprintf("Delete vault: proton-cli pass vaults delete %s", sid),
		"pass", "vaults", "delete", "--", sid)

	newName := name + "-renamed"
	runOK(t, "pass", "vaults", "rename", "--name", newName, sid)
	assertContains(t, runOK(t, "pass", "vaults", "list"), newName)
}

func TestPassLoginTOTPRoundTrips(t *testing.T) {
	name := testID() + "-totp"
	secret := "JBSWY3DPEHPK3PXP"
	ref := strings.TrimSpace(runOK(t, "pass", "items", "create", "--type", "login",
		"--name", name, "--username", "me@example.com",
		"--totp", "otpauth://totp/Example:me?secret="+secret+"&issuer=Example"))
	cleanupRun(t, fmt.Sprintf("Delete pass item: proton-cli pass items delete %s", ref),
		"pass", "items", "delete", "--", ref)

	assertContains(t, runOK(t, "pass", "items", "get", "--", ref), secret)
}
