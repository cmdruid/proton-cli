package tests

import (
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// ── public links ──

func tokenOf(t *testing.T, url string) string {
	t.Helper()
	i := strings.Index(url, "/urls/")
	if i < 0 {
		t.Fatalf("url %q has no /urls/ segment", url)
	}
	rest := url[i+len("/urls/"):]
	if h := strings.Index(rest, "#"); h >= 0 {
		return rest[:h]
	}
	return rest
}

func TestDriveShareLinkLifecycle(t *testing.T) {
	t.Parallel()
	folder := "/" + testID() + "-share"
	runOK(t, "drive", "folders", "create", folder)
	cleanupRun(t, fmt.Sprintf("Delete folder: proton-cli drive items delete --permanent %s", folder),
		"drive", "items", "delete", folder)

	url := strings.TrimSpace(runOK(t, "drive", "share", "link", folder))
	if !strings.Contains(url, "/urls/") {
		t.Fatalf("link stdout has no public URL: %q", url)
	}
	if !strings.Contains(url, "#") {
		t.Errorf("public link is missing the password fragment: %q", url)
	}

	status := runOK(t, "drive", "share", "get", folder)
	assertContains(t, status, "Public Link:")
	assertContains(t, status, tokenOf(t, url))

	runOK(t, "drive", "share", "unlink", folder)
	after := runOKStderr2(t, "drive", "share", "get", folder)
	assertField(t, after, "Shared:", "no")
}

// TestDriveShareLinkPublicHandshake guards the SRP-salt regression: a created
// link must be publicly resolvable. The public handshake fails auth (and the
// link 404s in a browser) if UrlPasswordSalt is not exactly 10 bytes, even
// though creation and status both still succeed.
func TestDriveShareLinkPublicHandshake(t *testing.T) {
	t.Parallel()
	folder := "/" + testID() + "-handshake"
	runOK(t, "drive", "folders", "create", folder)
	cleanupRun(t, fmt.Sprintf("Delete folder: proton-cli drive items delete --permanent %s", folder),
		"drive", "items", "delete", folder)

	url := strings.TrimSpace(runOK(t, "drive", "share", "link", folder))
	token := tokenOf(t, url)
	linkID := strings.TrimSpace(runJSON(t, "drive", "items", "get", folder)["link_id"].(string))

	info := runJSON(t, "api", "GET", "/drive/urls/"+token+"/info")
	if code, _ := info["Code"].(float64); int(code) != 1000 {
		t.Fatalf("public handshake Code = %v, want 1000", info["Code"])
	}
	salt, _ := info["UrlPasswordSalt"].(string)
	decoded, err := base64.StdEncoding.DecodeString(salt)
	if err != nil {
		t.Fatalf("UrlPasswordSalt not base64: %v", err)
	}
	if len(decoded) != 10 {
		t.Errorf("UrlPasswordSalt = %d bytes, want 10 (Proton SRP salt); a wrong length breaks public auth", len(decoded))
	}
	da, _ := info["DirectAccess"].(map[string]interface{})
	if da == nil || da["LinkID"] != linkID {
		t.Errorf("public link binds to %v, want LinkID %s", da["LinkID"], linkID)
	}
}

func TestDriveShareLinkIdempotent(t *testing.T) {
	t.Parallel()
	folder := "/" + testID() + "-idem"
	runOK(t, "drive", "folders", "create", folder)
	cleanupRun(t, fmt.Sprintf("Delete folder: proton-cli drive items delete --permanent %s", folder),
		"drive", "items", "delete", folder)

	first := strings.TrimSpace(runOK(t, "drive", "share", "link", folder))
	second := strings.TrimSpace(runOK(t, "drive", "share", "link", folder))
	if tokenOf(t, first) != tokenOf(t, second) {
		t.Errorf("link is not idempotent: %q vs %q", first, second)
	}
}

func TestDriveShareLinkExpires(t *testing.T) {
	t.Parallel()
	folder := "/" + testID() + "-exp"
	runOK(t, "drive", "folders", "create", folder)
	cleanupRun(t, fmt.Sprintf("Delete folder: proton-cli drive items delete --permanent %s", folder),
		"drive", "items", "delete", folder)

	link := runJSON(t, "drive", "share", "link", folder, "--expires", "7d")
	if link["expire_time"] == nil {
		t.Errorf("expected expire_time to be set, got %v", link["expire_time"])
	}
}

func TestDriveShareLinkPassword(t *testing.T) {
	t.Parallel()
	folder := "/" + testID() + "-pw"
	runOK(t, "drive", "folders", "create", folder)
	cleanupRun(t, fmt.Sprintf("Delete folder: proton-cli drive items delete --permanent %s", folder),
		"drive", "items", "delete", folder)

	// The record is the answer, so the custom password is reported there rather
	// than in the confirmation on stderr.
	stdout := runOK(t, "drive", "share", "link", folder, "--password", "hunter2")
	if !strings.Contains(stdout, "#") {
		t.Errorf("link should still carry a generated fragment: %q", stdout)
	}
	assertField(t, stdout, "Password:", "hunter2")
	assertField(t, runOK(t, "drive", "share", "get", folder), "Link Password:", "hunter2")
}

func TestDriveShareLinkDryRun(t *testing.T) {
	t.Parallel()
	folder := "/" + testID() + "-sharedry"
	runOK(t, "drive", "folders", "create", folder)
	cleanupRun(t, fmt.Sprintf("Delete folder: proton-cli drive items delete --permanent %s", folder),
		"drive", "items", "delete", folder)

	_, stderr := runOKStderr(t, "--dry-run", "drive", "share", "link", folder)
	assertContains(t, stderr, "Dry run")

	status := runOKStderr2(t, "drive", "share", "get", folder)
	assertField(t, status, "Shared:", "no")
}

// A link that already exists is changed rather than replaced, so the address
// people were given keeps working.
func TestDriveShareLinkUpdatesTheLinkItAlreadyHas(t *testing.T) {
	t.Parallel()
	folder := "/" + testID() + "-relink"
	runOK(t, "drive", "folders", "create", folder)
	cleanupRun(t, fmt.Sprintf("Delete folder: proton-cli drive items delete --permanent %s", folder),
		"drive", "items", "delete", folder)

	first := strings.TrimSpace(runOK(t, "drive", "share", "link", folder))
	updated := runOK(t, "drive", "share", "link", folder, "--password", "hunter2")

	assertField(t, updated, "Password:", "hunter2")
	if !strings.Contains(updated, tokenOf(t, first)) {
		t.Errorf("setting a password made a new link:\nwas %q\nnow %s", first, truncateOutput(updated))
	}
	assertField(t, runOK(t, "drive", "share", "get", folder), "Link Password:", "hunter2")
}

// ── members ──

func TestDriveShareAddNotProtonUser(t *testing.T) {
	t.Parallel()
	folder := "/" + testID() + "-member"
	runOK(t, "drive", "folders", "create", folder)
	cleanupRun(t, fmt.Sprintf("Delete folder: proton-cli drive items delete --permanent %s", folder),
		"drive", "items", "delete", folder)

	_, stderr, code := run(t, "drive", "share", "add", folder, "nobody-"+testID()+"@example.invalid")
	if code == 0 {
		t.Error("expected non-zero exit inviting a non-Proton address")
	}
	if !strings.Contains(stderr, "not a Proton user") {
		t.Errorf("expected 'not a Proton user' error, got: %s", stderr)
	}
}

func TestDriveShareAddDryRun(t *testing.T) {
	t.Parallel()
	folder := "/" + testID() + "-memberdry"
	runOK(t, "drive", "folders", "create", folder)
	cleanupRun(t, fmt.Sprintf("Delete folder: proton-cli drive items delete --permanent %s", folder),
		"drive", "items", "delete", folder)

	_, stderr := runOKStderr(t, "--dry-run", "drive", "share", "add", folder, "someone@example.invalid")
	assertContains(t, stderr, "Dry run")
}

// TestDriveShareMemberRoundTrip invites a real Proton address, verifies it shows
// as pending, then revokes it.
func TestDriveShareMemberRoundTrip(t *testing.T) {
	t.Parallel()
	lease(t, driveInvitations)
	invitee := secondaryEmail()
	folder := "/" + testID() + "-memberrt"
	tmp := t.TempDir()
	src := filepath.Join(tmp, "m.txt")
	_ = os.WriteFile(src, []byte("member round-trip"), 0644)
	runOK(t, "drive", "folders", "create", folder)
	// Permanent folder deletion cascades the share + any pending invitation.
	cleanupRun(t, fmt.Sprintf("Delete folder: proton-cli drive items delete --permanent %s", folder),
		"drive", "items", "delete", folder)
	runOK(t, "drive", "items", "upload", src, folder)
	file := folder + "/m.txt"

	runOK(t, "drive", "share", "add", file, invitee)
	status := runOK(t, "drive", "share", "get", file)
	assertContains(t, status, invitee)
	assertContains(t, status, "not yet accepted")

	runOK(t, "drive", "share", "remove", file, invitee)
	after := runOKStderr2(t, "drive", "share", "get", file)
	assertNotContains(t, after, invitee)
}

func TestDriveShareRemoveNotFound(t *testing.T) {
	t.Parallel()
	folder := "/" + testID() + "-rm"
	runOK(t, "drive", "folders", "create", folder)
	cleanupRun(t, fmt.Sprintf("Delete folder: proton-cli drive items delete --permanent %s", folder),
		"drive", "items", "delete", folder)

	_, _, code := run(t, "drive", "share", "remove", folder, "nobody@example.invalid")
	if code != 3 {
		t.Errorf("expected exit 3 removing an unknown member, got %d", code)
	}
}

// ── incoming invitations ──

func TestDriveInvitationsList(t *testing.T) {
	t.Parallel()
	// Single-account runs can't produce an incoming invite, so only assert the
	// command itself succeeds.
	_, _, code := run(t, "drive", "invitations", "list")
	if code != 0 {
		t.Errorf("invitations list should exit 0, got %d", code)
	}
}

func TestDriveInvitationsAcceptRejectDryRun(t *testing.T) {
	t.Parallel()
	_, stderr := runOKStderr(t, "--dry-run", "drive", "invitations", "accept", "some-invitation-id")
	assertContains(t, stderr, "Dry run")
	_, stderr = runOKStderr(t, "--dry-run", "drive", "invitations", "decline", "some-invitation-id")
	assertContains(t, stderr, "Dry run")
}

// runOKStderr2 joins stdout+stderr because status prints "Not shared." to
// stderr (via Info), not stdout.
func runOKStderr2(t *testing.T, args ...string) string {
	t.Helper()
	stdout, stderr := runOKStderr(t, args...)
	return stdout + stderr
}

// ── incoming invitations: two-account accept round-trip ──
//
// Needs the second account (the `secondary` profile): the primary shares
// an item with the alt, the alt accepts the invitation, and the primary then
// sees the alt as a member. Exercises the real accept crypto (session-key
// unwrap + signature), which the single-account tests can only dry-run.

func altInvitationIDs(t *testing.T) map[string]bool {
	t.Helper()
	set := map[string]bool{}
	for _, i := range runJSONArraySecondary(t, "drive", "invitations", "list") {
		if id, ok := i.(map[string]interface{})["invitation_id"].(string); ok {
			set[id] = true
		}
	}
	return set
}

// The other answer an invitation can be given. Accepting is the round trip above;
// declining has to leave the share without a member.
func TestDriveShareInvitationCanBeDeclined(t *testing.T) {
	t.Parallel()
	lease(t, driveInvitations)
	folder := "/" + testID() + "-decline"
	runOK(t, "drive", "folders", "create", folder)
	cleanupRun(t, fmt.Sprintf("Delete folder: proton-cli drive items delete --permanent %s", folder),
		"drive", "items", "delete", folder)

	before := altInvitationIDs(t)
	runOK(t, "drive", "share", "add", folder, secondaryEmail())
	cleanupRun(t, fmt.Sprintf("Revoke member: proton-cli drive share remove %s %s", folder, secondaryEmail()),
		"drive", "share", "remove", folder, secondaryEmail())

	var invID string
	waitFor(45*time.Second, 3*time.Second, func() bool {
		for id := range altInvitationIDs(t) {
			if !before[id] {
				invID = id
				return true
			}
		}
		return false
	})
	if invID == "" {
		t.Fatal("the second account never saw the invitation")
	}

	runOKSecondary(t, "drive", "invitations", "decline", invID)

	if altInvitationIDs(t)[invID] {
		t.Error("the invitation is still waiting for an answer after being declined")
	}
	st := runJSON(t, "drive", "share", "get", folder)
	members, _ := st["members"].([]interface{})
	if strings.Contains(fmt.Sprintf("%v", members), secondaryEmail()) {
		t.Errorf("declining made the second account a member anyway: %v", members)
	}
}

func TestDriveShareInvitationRoundTrip(t *testing.T) {
	t.Parallel()
	lease(t, driveInvitations)
	folder := "/" + testID() + "-share-rt"
	runOK(t, "drive", "folders", "create", folder)
	// Permanent delete cascades the share + membership.
	cleanupRun(t, fmt.Sprintf("Delete folder: proton-cli drive items delete --permanent %s", folder),
		"drive", "items", "delete", folder)

	before := altInvitationIDs(t)

	runOK(t, "drive", "share", "add", folder, secondaryEmail(), "--edit")
	cleanupRun(t, fmt.Sprintf("Revoke member: proton-cli drive share remove %s %s", folder, secondaryEmail()),
		"drive", "share", "remove", folder, secondaryEmail())

	// The alt sees the new pending invitation and accepts it.
	var invID string
	waitFor(45*time.Second, 3*time.Second, func() bool {
		for id := range altInvitationIDs(t) {
			if !before[id] {
				invID = id
				return true
			}
		}
		return false
	})
	if invID == "" {
		t.Fatal("alt did not receive the share invitation")
	}
	runOKSecondary(t, "drive", "invitations", "accept", invID)

	// The primary now sees the alt as a member, not a pending invitee.
	var members string
	waitFor(30*time.Second, 3*time.Second, func() bool {
		st := runJSON(t, "drive", "share", "get", folder)
		ms, _ := st["members"].([]interface{})
		members = fmt.Sprintf("%v", ms)
		return strings.Contains(members, secondaryEmail())
	})
	if !strings.Contains(members, secondaryEmail()) {
		t.Errorf("alt is not listed as a member after accepting; members=%s", members)
	}
}
