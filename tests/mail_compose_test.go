package tests

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Drafts, replies and forwards all go through the same compose path, so these
// tests cover it end to end: a draft that is written, edited and sent; a reply
// that threads onto its parent; and a forward whose attachment arrives intact at
// the second account.

// ── drafts ──

func TestMailDraftsLifecycle(t *testing.T) {
	t.Parallel()
	subject := testID() + "-draft"

	stdout := runOK(t, "mail", "drafts", "create",
		"--to", selfEmail(), "--subject", subject, "--body", "first version")
	id := strings.TrimSpace(stdout)
	if !looksLikeID(id) {
		t.Fatalf("drafts create should print a bare ID, got %q", stdout)
	}
	cleanupRun(t, "Delete draft: proton-cli mail drafts delete "+id,
		"mail", "messages", "delete", "--", id)

	// It shows up as a draft, listed by the dedicated command.
	list := runJSON(t, "mail", "drafts", "list", "--page-size", "50")
	drafts, _ := list["drafts"].([]interface{})
	found := false
	for _, d := range drafts {
		if d.(map[string]interface{})["id"] == id {
			found = true
		}
	}
	if !found {
		t.Error("the new draft did not appear in `mail drafts list`")
	}

	// Editing replaces only what was passed.
	runOK(t, "mail", "drafts", "update", "--body", "second version", "--", id)
	read := runOK(t, "mail", "messages", "get", "--", id)
	assertContains(t, read, "second version")
	assertNotContains(t, read, "first version")
	assertField(t, read, "Subject:", subject)

	runOK(t, "mail", "drafts", "update", "--subject", subject+"-renamed", "--", id)
	read = runOK(t, "mail", "messages", "get", "--", id)
	assertContains(t, read, subject+"-renamed")
	assertContains(t, read, "second version")

	// Sending it delivers the stored body.
	runOK(t, "mail", "drafts", "send", "--", id)
	inboxID := findMessage(t, "inbox", subject+"-renamed")
	if inboxID == "" {
		t.Fatal("the sent draft never arrived")
	}
	cleanupRun(t, "Delete inbox copy: proton-cli mail messages delete "+inboxID,
		"mail", "messages", "delete", "--", inboxID)
	assertContains(t, runOK(t, "mail", "messages", "get", "--", inboxID), "second version")
}

func TestMailDraftsCreateDryRunCreatesNothing(t *testing.T) {
	t.Parallel()
	subject := testID() + "-draft-dry"
	_, stderr := runOKStderr(t, "--dry-run", "mail", "drafts", "create",
		"--to", selfEmail(), "--subject", subject, "--body", "x")
	assertContains(t, stderr, "Dry run")
	if messageIDInFolder("drafts", subject) != "" {
		t.Error("--dry-run created a draft")
	}
}

func TestMailDraftsAttachAndDetach(t *testing.T) {
	t.Parallel()
	subject := testID() + "-draft-attach"
	dir := t.TempDir()
	path := filepath.Join(dir, "note.txt")
	if err := os.WriteFile(path, []byte("draft attachment"), 0o600); err != nil {
		t.Fatal(err)
	}

	id := strings.TrimSpace(runOK(t, "mail", "drafts", "create",
		"--to", selfEmail(), "--subject", subject, "--body", "see attached", "--attach", path))
	cleanupRun(t, "Delete draft: proton-cli mail messages delete "+id,
		"mail", "messages", "delete", "--", id)

	assertContains(t, runOK(t, "mail", "messages", "attachments", "list", id), "note.txt")

	// --detach takes the file name, not just an ID.
	runOK(t, "mail", "drafts", "update", "--detach", "note.txt", "--", id)
	assertNotContains(t, runOK(t, "mail", "messages", "attachments", "list", id), "note.txt")
}

func TestMailDraftsEditRejectsASentMessage(t *testing.T) {
	t.Parallel()
	_, _, subject := plainMail(t)
	// REF resolution for drafts is scoped to the Drafts folder, so a sent
	// message's subject must not resolve.
	_, stderr, code := run(t, "mail", "drafts", "update", "--body", "nope", subject)
	// Exit 4 means something in Drafts answered to this subject, and which
	// messages those were is the whole story - stderr names them.
	if code != 3 {
		t.Errorf("expected exit 3 for a subject that is not a draft, got %d\nstderr: %s",
			code, truncateOutput(stderr))
	}
}

func TestMailDraftsSendRequiresARecipient(t *testing.T) {
	t.Parallel()
	subject := testID() + "-draft-norecipient"
	id := strings.TrimSpace(runOK(t, "mail", "drafts", "create", "--subject", subject, "--body", "x"))
	cleanupRun(t, "Delete draft: proton-cli mail messages delete "+id,
		"mail", "messages", "delete", "--", id)

	_, stderr, code := run(t, "mail", "drafts", "send", "--", id)
	if code == 0 {
		t.Error("sending a draft with no recipients should fail")
	}
	assertContains(t, stderr, "no recipients")
}

// ── reply ──

func TestMailMessagesReply(t *testing.T) {
	t.Parallel()
	subject := testID() + "-reply-parent"
	parentID := sendTestMail(t, subject)

	stdout := runOK(t, "mail", "messages", "reply", "--body", "My answer here.", "--", parentID)
	replyID := strings.TrimSpace(stdout)
	if !looksLikeID(replyID) {
		t.Fatalf("reply should print a bare ID, got %q", stdout)
	}
	cleanupRun(t, "Delete reply: proton-cli mail messages delete "+replyID,
		"mail", "messages", "delete", "--", replyID)

	body := runOK(t, "mail", "messages", "get", "--", replyID)
	assertField(t, body, "Subject:", "Re: "+subject)
	assertContains(t, body, "My answer here.")
	assertContains(t, body, "wrote:")
	assertContains(t, body, "> Integration test body")

	// --strip-quotes removes exactly the quote we just wrote.
	stripped := runOK(t, "mail", "messages", "get", "--strip-quotes", "--", replyID)
	assertContains(t, stripped, "My answer here.")
	assertNotContains(t, stripped, "> Integration test body")

	// The parent is flagged as replied to, which is what ParentID + Action buy.
	data := runJSON(t, "api", "GET", "/mail/v4/messages/"+parentID)
	msg, _ := data["Message"].(map[string]interface{})
	if replied, _ := msg["IsReplied"].(float64); replied != 1 {
		t.Errorf("parent IsReplied = %v, want 1", msg["IsReplied"])
	}

	// The inbox copy needs cleaning up too.
	if inbox := findMessage(t, "inbox", "Re: "+subject); inbox != "" && inbox != replyID {
		cleanupRun(t, "Delete reply inbox copy: proton-cli mail messages delete "+inbox,
			"mail", "messages", "delete", "--", inbox)
	}
}

func TestMailMessagesReplyNoQuote(t *testing.T) {
	t.Parallel()
	msgID, _, _ := plainMail(t)
	id := strings.TrimSpace(runOK(t, "mail", "messages", "reply",
		"--body", "Terse.", "--no-quote", "--", msgID))
	cleanupRun(t, "Delete reply: proton-cli mail messages delete "+id,
		"mail", "messages", "delete", "--", id)

	body := runOK(t, "mail", "messages", "get", "--", id)
	assertContains(t, body, "Terse.")
	assertNotContains(t, body, "wrote:")
}

func TestMailMessagesReplyAsDraft(t *testing.T) {
	t.Parallel()
	msgID, _, subject := plainMail(t)
	stdout, stderr := runOKStderr(t, "mail", "messages", "reply",
		"--body", "Later.", "--draft", "--", msgID)
	id := strings.TrimSpace(stdout)
	if !looksLikeID(id) {
		t.Fatalf("--draft should print the draft ID, got %q", stdout)
	}
	cleanupRun(t, "Delete reply draft: proton-cli mail messages delete "+id,
		"mail", "messages", "delete", "--", id)
	assertContains(t, stderr, "draft")

	// It is a draft, not a sent message.
	if messageIDInFolder("drafts", "Re: "+subject) == "" {
		t.Error("--draft did not leave the reply in Drafts")
	}
	assertContains(t, runOK(t, "mail", "messages", "get", "--", id), "Later.")
}

func TestMailMessagesReplyDryRun(t *testing.T) {
	t.Parallel()
	msgID, _, _ := plainMail(t)
	_, stderr := runOKStderr(t, "--dry-run", "mail", "messages", "reply",
		"--body", "x", "--", msgID)
	// A reply is a send, so it reports as one - naming the message it would put
	// on the wire rather than the verb that composed it.
	assertContains(t, stderr, "would send message")
	assertContains(t, stderr, "Re: ")
}

// ── forward ──

func TestMailMessagesForwardCarriesAttachmentsToTheAltAccount(t *testing.T) {
	t.Parallel()
	msgID, _, attName := sharedAttachment(t)
	marker := testID() + "-forwarded"

	fwdID := strings.TrimSpace(runOK(t, "mail", "messages", "forward",
		"--to", secondaryEmail(), "--body", marker, "--", msgID))
	cleanupRun(t, "Delete forward: proton-cli mail messages delete "+fwdID,
		"mail", "messages", "delete", "--", fwdID)

	// The sent copy carries the forwarded headers and the original's attachment.
	sent := runOK(t, "mail", "messages", "get", "--", fwdID)
	assertContains(t, sent, marker)
	assertContains(t, sent, "Forwarded Message")
	assertContains(t, sent, attName)

	// The second account receives it, attachment and all.
	var recvID string
	waitFor(45*time.Second, 3*time.Second, func() bool {
		recvID = secondaryMailContaining(t, selfEmail(), marker)
		return recvID != ""
	})
	if recvID == "" {
		t.Fatal("the second account never received the forward")
	}
	cleanupRunSecondary(t, "Delete received forward (secondary): proton-cli --profile secondary mail messages delete "+recvID,
		"mail", "messages", "delete", recvID)

	atts := runOKSecondary(t, "mail", "messages", "attachments", "list", recvID)
	assertContains(t, atts, attName)

	// And the bytes survived the re-keying rather than the upload.
	dir := t.TempDir()
	runOKSecondary(t, "mail", "messages", "attachments", "download", "--output-dir", dir, recvID)
	got, err := os.ReadFile(filepath.Join(dir, attName))
	if err != nil {
		t.Fatalf("read forwarded attachment: %v", err)
	}
	if len(got) == 0 {
		t.Error("the forwarded attachment arrived empty")
	}
}

func TestMailMessagesForwardWithoutAttachments(t *testing.T) {
	t.Parallel()
	msgID, _, attName := sharedAttachment(t)
	subject := testID() + "-fwd-noatt"

	id := strings.TrimSpace(runOK(t, "mail", "messages", "forward",
		"--to", selfEmail(), "--body", subject, "--no-attachments", "--", msgID))
	cleanupRun(t, "Delete forward: proton-cli mail messages delete "+id,
		"mail", "messages", "delete", "--", id)

	for _, row := range runJSONArray(t, "mail", "messages", "attachments", "list", id) {
		a, _ := row.(map[string]interface{})
		if n, _ := a["name"].(string); n == attName {
			t.Errorf("--no-attachments still carried %s", attName)
		}
	}
}

func TestMailMessagesForwardRequiresTo(t *testing.T) {
	t.Parallel()
	msgID, _, _ := plainMail(t)
	_, stderr, code := run(t, "mail", "messages", "forward", "--body", "x", "--", msgID)
	if code == 0 {
		t.Error("forwarding without --to should fail")
	}
	assertContains(t, stderr, "--to is required")
}

// ── sender selection ──

func TestMailSendFromRejectsAnAddressYouDoNotOwn(t *testing.T) {
	t.Parallel()
	_, stderr, code := run(t, "mail", "messages", "send",
		"--from", "someone@not-your-account.invalid",
		"--to", selfEmail(), "--subject", testID(), "--body", "x")
	if code != 3 {
		t.Errorf("expected exit 3 for an unknown --from, got %d", code)
	}
	assertContains(t, stderr, "can send mail")
	assertContains(t, stderr, selfEmail())
}

func TestMailSendFromAcceptsYourOwnAddress(t *testing.T) {
	t.Parallel()
	subject := testID() + "-from"
	_, stderr := runOKStderr(t, "--dry-run", "mail", "messages", "send",
		"--from", selfEmail(), "--to", selfEmail(), "--subject", subject, "--body", "x")
	assertContains(t, stderr, "would send message")
	assertContains(t, stderr, subject)
}

// ── signature ──

// Signatures are applied automatically, matching the web client, so a message
// composed with one set carries it and --no-signature suppresses it.
func TestMailSignatureIsAppliedAndSuppressible(t *testing.T) {
	t.Parallel()
	lease(t, addressIdentity)
	addrID := primaryAddressID(t)
	original := addressSignature(t, addrID)
	marker := testID() + "-sig"
	runOK(t, "mail", "settings", "addresses", "update", "--signature", marker, "--", addrID)
	cleanup(t, "Restore the address signature", func() error {
		args := []string{"mail", "settings", "addresses", "update", "--html", "--signature", original, "--", addrID}
		if strings.TrimSpace(original) == "" {
			args = []string{"mail", "settings", "addresses", "update", "--clear-signature", "--", addrID}
		}
		_, stderr, code := run(t, args...)
		if code != 0 {
			return fmt.Errorf("exit %d: %s", code, strings.TrimSpace(stderr))
		}
		return nil
	})

	subject := testID() + "-sig-send"
	id := strings.TrimSpace(runOK(t, "mail", "drafts", "create",
		"--to", selfEmail(), "--subject", subject, "--body", "Body text."))
	cleanupRun(t, "Delete draft: proton-cli mail messages delete "+id,
		"mail", "messages", "delete", "--", id)
	assertContains(t, runOK(t, "mail", "messages", "get", "--", id), marker)

	bare := strings.TrimSpace(runOK(t, "mail", "drafts", "create",
		"--to", selfEmail(), "--subject", subject+"-bare", "--body", "Body text.", "--no-signature"))
	cleanupRun(t, "Delete draft: proton-cli mail messages delete "+bare,
		"mail", "messages", "delete", "--", bare)
	assertNotContains(t, runOK(t, "mail", "messages", "get", "--", bare), marker)
}

// primaryAddressID returns the account's first address ID.
func primaryAddressID(t *testing.T) string {
	t.Helper()
	raw := runOK(t, "--full-ids", "mail", "settings", "addresses", "list", "--output", "json")
	var env struct {
		Addresses []struct {
			ID    string `json:"id"`
			Email string `json:"email"`
		} `json:"addresses"`
	}
	if err := json.Unmarshal([]byte(raw), &env); err != nil || len(env.Addresses) == 0 {
		t.Fatalf("could not read the address list: %v\n%s", err, truncateOutput(raw))
	}
	for _, a := range env.Addresses {
		if a.Email == selfEmail() {
			return a.ID
		}
	}
	return env.Addresses[0].ID
}

func addressSignature(t *testing.T, addrID string) string {
	t.Helper()
	raw := runOK(t, "mail", "settings", "addresses", "get", "--output", "json", "--", addrID)
	var a struct {
		Signature string `json:"signature"`
	}
	if err := json.Unmarshal([]byte(raw), &a); err != nil {
		t.Fatalf("could not read the address: %v", err)
	}
	return a.Signature
}
