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

	status := runOK(t, "drive", "share", "status", folder)
	assertContains(t, status, "Public links:")
	assertContains(t, status, tokenOf(t, url))

	runOK(t, "drive", "share", "unlink", folder)
	after := runOKStderr2(t, "drive", "share", "status", folder)
	assertContains(t, after, "Not shared.")
}

// TestDriveShareLinkPublicHandshake guards the SRP-salt regression: a created
// link must be publicly resolvable. The public handshake fails auth (and the
// link 404s in a browser) if UrlPasswordSalt is not exactly 10 bytes, even
// though creation and status both still succeed.
func TestDriveShareLinkPublicHandshake(t *testing.T) {
	folder := "/" + testID() + "-handshake"
	runOK(t, "drive", "folders", "create", folder)
	cleanupRun(t, fmt.Sprintf("Delete folder: proton-cli drive items delete --permanent %s", folder),
		"drive", "items", "delete", folder)

	url := strings.TrimSpace(runOK(t, "drive", "share", "link", folder))
	token := tokenOf(t, url)
	linkID := strings.TrimSpace(runJSON(t, "drive", "items", "info", folder)["link_id"].(string))

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
	folder := "/" + testID() + "-pw"
	runOK(t, "drive", "folders", "create", folder)
	cleanupRun(t, fmt.Sprintf("Delete folder: proton-cli drive items delete --permanent %s", folder),
		"drive", "items", "delete", folder)

	stdout, stderr := runOKStderr(t, "drive", "share", "link", folder, "--password", "hunter2")
	if !strings.Contains(strings.TrimSpace(stdout), "#") {
		t.Errorf("link should still carry a generated fragment: %q", stdout)
	}
	if !strings.Contains(stderr, "hunter2") {
		t.Errorf("expected the custom password reported on stderr, got: %q", stderr)
	}
}

func TestDriveShareLinkDryRun(t *testing.T) {
	folder := "/" + testID() + "-sharedry"
	runOK(t, "drive", "folders", "create", folder)
	cleanupRun(t, fmt.Sprintf("Delete folder: proton-cli drive items delete --permanent %s", folder),
		"drive", "items", "delete", folder)

	_, stderr := runOKStderr(t, "--dry-run", "drive", "share", "link", folder)
	assertContains(t, stderr, "dry-run")

	status := runOKStderr2(t, "drive", "share", "status", folder)
	assertContains(t, status, "Not shared.")
}

// ── members ──

func TestDriveShareAddNotProtonUser(t *testing.T) {
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
	folder := "/" + testID() + "-memberdry"
	runOK(t, "drive", "folders", "create", folder)
	cleanupRun(t, fmt.Sprintf("Delete folder: proton-cli drive items delete --permanent %s", folder),
		"drive", "items", "delete", folder)

	_, stderr := runOKStderr(t, "--dry-run", "drive", "share", "add", folder, "someone@example.invalid")
	assertContains(t, stderr, "dry-run")
}

// TestDriveShareMemberRoundTrip invites a real Proton address, verifies it shows
// as pending, then revokes it.
func TestDriveShareMemberRoundTrip(t *testing.T) {
	invitee := altEmail()
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
	status := runOK(t, "drive", "share", "status", file)
	assertContains(t, status, invitee)
	assertContains(t, status, "pending")

	runOK(t, "drive", "share", "remove", file, invitee)
	after := runOKStderr2(t, "drive", "share", "status", file)
	assertNotContains(t, after, invitee)
}

func TestDriveShareRemoveNotFound(t *testing.T) {
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
	// Single-account runs can't produce an incoming invite, so only assert the
	// command itself succeeds.
	_, _, code := run(t, "drive", "invitations", "list")
	if code != 0 {
		t.Errorf("invitations list should exit 0, got %d", code)
	}
}

func TestDriveInvitationsAcceptRejectDryRun(t *testing.T) {
	_, stderr := runOKStderr(t, "--dry-run", "drive", "invitations", "accept", "some-invitation-id")
	assertContains(t, stderr, "dry-run")
	_, stderr = runOKStderr(t, "--dry-run", "drive", "invitations", "reject", "some-invitation-id")
	assertContains(t, stderr, "dry-run")
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
// Needs the "Proton Alt" second account (the `alt` profile): the primary shares
// an item with the alt, the alt accepts the invitation, and the primary then
// sees the alt as a member. Exercises the real accept crypto (session-key
// unwrap + signature), which the single-account tests can only dry-run.

func altInvitationIDs(t *testing.T) map[string]bool {
	t.Helper()
	set := map[string]bool{}
	for _, i := range runJSONArray(t, alt("drive", "invitations", "list")...) {
		if id, ok := i.(map[string]interface{})["invitation_id"].(string); ok {
			set[id] = true
		}
	}
	return set
}

func TestDriveShareInvitationRoundTrip(t *testing.T) {
	folder := "/" + testID() + "-share-rt"
	runOK(t, "drive", "folders", "create", folder)
	// Permanent delete cascades the share + membership.
	cleanupRun(t, fmt.Sprintf("Delete folder: proton-cli drive items delete --permanent %s", folder),
		"drive", "items", "delete", folder)

	before := altInvitationIDs(t)

	runOK(t, "drive", "share", "add", folder, altEmail(), "--edit")
	cleanupRun(t, fmt.Sprintf("Revoke member: proton-cli drive share remove %s %s", folder, altEmail()),
		"drive", "share", "remove", folder, altEmail())

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
	runOK(t, alt("drive", "invitations", "accept", invID)...)

	// The primary now sees the alt as a member, not a pending invitee.
	var members string
	waitFor(30*time.Second, 3*time.Second, func() bool {
		st := runJSON(t, "drive", "share", "status", folder)
		ms, _ := st["members"].([]interface{})
		members = fmt.Sprintf("%v", ms)
		return strings.Contains(members, altEmail())
	})
	if !strings.Contains(members, altEmail()) {
		t.Errorf("alt is not listed as a member after accepting; members=%s", members)
	}
}
