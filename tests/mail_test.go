package tests

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"
)

// ── mail messages list ──

func TestMailMessagesList(t *testing.T) {
	skipIfNoCredentials(t)
	stdout := runOK(t, "mail", "messages", "list")
	assertContains(t, stdout, "ID")
	assertContains(t, stdout, "FROM")
	assertContains(t, stdout, "SUBJECT")
}

func TestMailMessagesListSent(t *testing.T) {
	skipIfNoCredentials(t)
	runOK(t, "mail", "messages", "list", "--folder", "sent")
}

func TestMailMessagesListJSONFieldNames(t *testing.T) {
	skipIfNoCredentials(t)
	data := runJSON(t, "mail", "messages", "list", "--page-size", "1")
	msgs, ok := data["messages"].([]interface{})
	if !ok {
		t.Fatal("expected messages array")
	}
	if len(msgs) > 0 {
		m := msgs[0].(map[string]interface{})
		for _, field := range []string{"id", "subject", "from_address", "num_attachments", "time"} {
			if _, has := m[field]; !has {
				t.Errorf("expected json field %q (snake_case), got keys: %v", field, keysOf(m))
			}
		}
	}
}

func TestMailMessagesListPageSize(t *testing.T) {
	skipIfNoCredentials(t)
	data := runJSON(t, "mail", "messages", "list", "--page-size", "3")
	msgs := data["messages"].([]interface{})
	if len(msgs) > 3 {
		t.Errorf("expected at most 3 messages, got %d", len(msgs))
	}
}

func TestMailMessagesListUnreadFlag(t *testing.T) {
	skipIfNoCredentials(t)
	runOK(t, "mail", "messages", "list", "--unread")
}

// ── list footer / json shape ──

func TestMailMessagesListFooterSinglePage(t *testing.T) {
	skipIfNoCredentials(t)
	// Use 150 (Proton's documented max for messages list); 500 is
	// rejected as "Invalid page size parameter".
	_, stderr := runOKStderr(t, "mail", "messages", "list", "--page-size", "150")
	last := lastNonEmpty(stderr)
	if !strings.Contains(last, "single page") && !strings.Contains(last, "Showing") {
		t.Errorf("expected single-page or showing footer, got: %q", last)
	}
	if strings.Contains(last, "page 0") {
		t.Errorf("footer still has '(page 0)' wording: %q", last)
	}
}

func TestMailMessagesListFooterMidPagination(t *testing.T) {
	skipIfNoCredentials(t)
	_, stderr := runOKStderr(t, "mail", "messages", "list", "--page-size", "1")
	last := lastNonEmpty(stderr)
	// Either mid-pagination ("Pass --page 1") or last/single-page if the
	// account has ≤ 1 messages. Pin the substring that's present in the
	// common case.
	if !strings.Contains(last, "--page 1") && !strings.Contains(last, "single page") && !strings.Contains(last, "last page") {
		t.Errorf("expected pagination footer, got: %q", last)
	}
}

func TestMailMessagesListJSONPaginationFields(t *testing.T) {
	skipIfNoCredentials(t)
	data := runJSON(t, "mail", "messages", "list", "--page-size", "1")
	for _, key := range []string{"total", "page", "page_size", "has_more", "messages"} {
		if _, ok := data[key]; !ok {
			t.Errorf("expected JSON field %q, got keys: %v", key, keysOf(data))
		}
	}
}

func TestMailMessagesSearchFooterNoPageZero(t *testing.T) {
	skipIfNoCredentials(t)
	_, stderr := runOKStderr(t, "mail", "messages", "search", "--keyword", "proton", "--limit", "5")
	last := lastNonEmpty(stderr)
	if strings.Contains(last, "page 0") {
		t.Errorf("search footer still has '(page 0)': %q", last)
	}
	// Must look like one of the SearchFooter variants.
	if !strings.Contains(last, "results") && !strings.Contains(last, "No results") {
		t.Errorf("expected search-result footer, got: %q", last)
	}
}

func TestMailMessagesSearchEmptyFooter(t *testing.T) {
	skipIfNoCredentials(t)
	_, stderr := runOKStderr(t, "mail", "messages", "search",
		"--keyword", "xyz-no-match-"+testID())
	last := lastNonEmpty(stderr)
	if !strings.Contains(last, "No results") {
		t.Errorf("expected 'No results.' on empty search, got: %q", last)
	}
}

func lastNonEmpty(s string) string {
	lines := strings.Split(s, "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		line := strings.TrimSpace(lines[i])
		if line != "" {
			return line
		}
	}
	return ""
}

// ── mail messages search ──

func TestMailMessagesSearch(t *testing.T) {
	skipIfNoCredentials(t)
	runOK(t, "mail", "messages", "search", "--keyword", "proton")
}

func TestMailMessagesSearchFrom(t *testing.T) {
	skipIfNoCredentials(t)
	runOK(t, "mail", "messages", "search", "--from", selfEmail())
}

func TestMailMessagesSearchDateRange(t *testing.T) {
	skipIfNoCredentials(t)
	runOK(t, "mail", "messages", "search", "--after", "2020-01-01", "--before", "2099-12-31")
}

func TestMailMessagesSearchEmpty(t *testing.T) {
	skipIfNoCredentials(t)
	_, _, code := run(t, "mail", "messages", "search", "--keyword", "xyz-nothing-xxxyyy-"+testID())
	if code != 0 {
		t.Fatalf("search with no results should exit 0, got %d", code)
	}
}

// ── --from / --to zero-result hint ──

func TestMailSearchFromZeroResultsHint(t *testing.T) {
	skipIfNoCredentials(t)
	needle := "no-such-sender-" + testID()
	_, stderr := runOKStderr(t, "mail", "messages", "search", "--from", needle)
	if !strings.Contains(stderr, "Hint:") {
		t.Errorf("expected stderr to contain 'Hint:', got: %s", stderr)
	}
	if !strings.Contains(stderr, "--keyword "+needle) {
		t.Errorf("expected stderr to contain '--keyword %s', got: %s", needle, stderr)
	}
}

func TestMailSearchToZeroResultsHint(t *testing.T) {
	skipIfNoCredentials(t)
	needle := "no-such-rcpt-" + testID()
	_, stderr := runOKStderr(t, "mail", "messages", "search", "--to", needle)
	if !strings.Contains(stderr, "Hint:") {
		t.Errorf("expected stderr to contain 'Hint:', got: %s", stderr)
	}
	if !strings.Contains(stderr, "--keyword "+needle) {
		t.Errorf("expected stderr to contain '--keyword %s', got: %s", needle, stderr)
	}
}

func TestMailSearchFromKeywordSuppressesHint(t *testing.T) {
	skipIfNoCredentials(t)
	needle := "impossible-" + testID()
	_, stderr := runOKStderr(t, "mail", "messages", "search",
		"--from", needle, "--keyword", "alsoimpossible-"+testID())
	if strings.Contains(stderr, "Hint:") {
		t.Errorf("hint should be suppressed when --keyword is set; got: %s", stderr)
	}
}

func TestMailSearchFromHitsNoHint(t *testing.T) {
	skipIfNoCredentials(t)
	subject := testID() + "-from-hint"
	sendTestMail(t, subject)
	// Self-mail: --from selfEmail() should match. May take a beat to index.
	var stderr string
	for attempt := 0; attempt < 8; attempt++ {
		_, s := runOKStderr(t, "mail", "messages", "search",
			"--from", selfEmail(), "--limit", "5")
		stderr = s
		if !strings.Contains(s, "Hint:") {
			return
		}
		time.Sleep(2 * time.Second)
	}
	t.Errorf("hint fired on a successful --from query; stderr: %s", stderr)
}

func TestMailSearchFromQuietSuppressesHint(t *testing.T) {
	skipIfNoCredentials(t)
	_, stderr := runOKStderr(t, "--quiet", "mail", "messages", "search",
		"--from", "no-such-sender-"+testID())
	if strings.Contains(stderr, "Hint:") {
		t.Errorf("--quiet should suppress 'Hint:'; got: %s", stderr)
	}
}

func TestMailConversationsSearchFromZeroResultsHint(t *testing.T) {
	skipIfNoCredentials(t)
	needle := "no-such-sender-" + testID()
	_, stderr := runOKStderr(t, "mail", "conversations", "search", "--from", needle)
	if !strings.Contains(stderr, "Hint:") {
		t.Errorf("expected stderr to contain 'Hint:', got: %s", stderr)
	}
	if !strings.Contains(stderr, "proton-cli mail conversations search --keyword "+needle) {
		t.Errorf("expected conversations-tree redirect, got: %s", stderr)
	}
}

// ── send / read / REF search ──

func TestMailMessagesSendAndReadText(t *testing.T) {
	skipIfNoCredentials(t)
	subject := testID() + "-send-read"
	msgID := sendTestMail(t, subject)

	// Default --format text: human-readable, fields on stderr-safe stdout
	stdout := runOK(t, "mail", "messages", "read", "--", msgID)
	assertContains(t, stdout, subject)
	assertContains(t, stdout, selfEmail())
	assertField(t, stdout, "Subject:", subject)
}

func TestMailMessagesReadByRef(t *testing.T) {
	skipIfNoCredentials(t)
	subject := testID() + "-ref"
	sendTestMail(t, subject)

	// Proton's search index is populated asynchronously, so the message may
	// show up in list (used by sendTestMail) a few seconds before it shows up
	// in the keyword-search endpoint that REF resolution uses. Retry with
	// backoff instead of hard-failing on the first attempt.
	var stdout, lastStderr string
	var lastCode int
	for attempt := 0; attempt < 8; attempt++ {
		out, stderr, code := run(t, "mail", "messages", "read", subject)
		if code == 0 {
			stdout = out
			break
		}
		lastStderr = stderr
		lastCode = code
		time.Sleep(3 * time.Second)
	}
	if stdout == "" {
		t.Fatalf("REF resolution did not index within timeout (exit %d): %s", lastCode, lastStderr)
	}
	assertContains(t, stdout, subject)
}

func TestMailMessagesReadFormatRaw(t *testing.T) {
	skipIfNoCredentials(t)
	subject := testID() + "-raw"
	msgID := sendTestMail(t, subject)

	stdout := runOK(t, "mail", "messages", "read", "--format", "raw", "--", msgID)
	assertContains(t, stdout, subject)
}

func TestMailMessagesReadFormatInvalid(t *testing.T) {
	skipIfNoCredentials(t)
	subject := testID() + "-badfmt"
	msgID := sendTestMail(t, subject)

	_, stderr, code := run(t, "mail", "messages", "read", "--format", "wut", "--", msgID)
	if code == 0 {
		t.Error("expected non-zero exit for unknown --format")
	}
	assertContains(t, stderr, "unknown --format")
}

func TestMailMessagesReadNotFound(t *testing.T) {
	skipIfNoCredentials(t)
	_, _, code := run(t, "mail", "messages", "read", "no-such-msg-"+testID())
	if code != 3 {
		t.Errorf("expected exit 3 (not-found), got %d", code)
	}
}

// ── mark / star / unstar ──

func TestMailMessagesMarkReadUnread(t *testing.T) {
	skipIfNoCredentials(t)
	subject := testID() + "-mark"
	msgID := sendTestMail(t, subject)

	runOK(t, "mail", "messages", "mark", "unread", "--", msgID)
	data := runJSON(t, "mail", "messages", "list", "--unread", "--page-size", "50")
	msgs := data["messages"].([]interface{})
	found := false
	for _, m := range msgs {
		if m.(map[string]interface{})["id"].(string) == msgID {
			found = true
			break
		}
	}
	if !found {
		t.Error("message should be in --unread list after mark unread")
	}

	runOK(t, "mail", "messages", "mark", "read", "--", msgID)
	data = runJSON(t, "mail", "messages", "list", "--unread", "--page-size", "50")
	msgs = data["messages"].([]interface{})
	for _, m := range msgs {
		if m.(map[string]interface{})["id"].(string) == msgID {
			t.Error("message should NOT be in --unread list after mark read")
		}
	}
}

func TestMailMessagesStarUnstar(t *testing.T) {
	skipIfNoCredentials(t)
	subject := testID() + "-star"
	msgID := sendTestMail(t, subject)

	runOK(t, "mail", "messages", "star", "--", msgID)
	data := runJSON(t, "mail", "messages", "list", "--folder", "starred", "--page-size", "50")
	msgs := data["messages"].([]interface{})
	found := false
	for _, m := range msgs {
		if m.(map[string]interface{})["id"].(string) == msgID {
			found = true
			break
		}
	}
	if !found {
		t.Error("message should appear in starred folder after star")
	}

	runOK(t, "mail", "messages", "unstar", "--", msgID)
}

// ── move / trash with --dest ──

func TestMailMessagesMoveDest(t *testing.T) {
	skipIfNoCredentials(t)
	subject := testID() + "-move"
	msgID := sendTestMail(t, subject)

	runOK(t, "mail", "messages", "move", "--dest", "archive", "--", msgID)
	data := runJSON(t, "mail", "messages", "list", "--folder", "archive", "--page-size", "50")
	msgs := data["messages"].([]interface{})
	found := false
	for _, m := range msgs {
		if m.(map[string]interface{})["id"].(string) == msgID {
			found = true
			break
		}
	}
	if !found {
		t.Error("message should appear in archive after --dest archive")
	}

	runOK(t, "mail", "messages", "move", "--dest", "inbox", "--", msgID)
}

func TestMailMessagesTrash(t *testing.T) {
	skipIfNoCredentials(t)
	subject := testID() + "-trash"
	msgID := sendTestMail(t, subject)

	runOK(t, "mail", "messages", "trash", "--", msgID)
	data := runJSON(t, "mail", "messages", "list", "--page-size", "50")
	msgs := data["messages"].([]interface{})
	for _, m := range msgs {
		if m.(map[string]interface{})["id"].(string) == msgID {
			t.Error("trashed message should not appear in inbox")
		}
	}
	// put it back so cleanup can delete
	runOK(t, "mail", "messages", "move", "--dest", "inbox", "--", msgID)
}

// ── batch filters (all dry-run so nothing is actually mutated) ──

func TestMailBatchTrashDryRunUnread(t *testing.T) {
	skipIfNoCredentials(t)
	_, stderr := runOKStderr(t, "--dry-run", "mail", "messages", "trash", "--unread", "--limit", "5")
	assertContains(t, stderr, "dry-run")
}

func TestMailBatchTrashDryRunOlderThan(t *testing.T) {
	skipIfNoCredentials(t)
	_, stderr := runOKStderr(t, "--dry-run", "mail", "messages", "trash", "--older-than", "365d", "--from", "noreply", "--limit", "5")
	assertContains(t, stderr, "dry-run")
}

func TestMailBatchRequiresInput(t *testing.T) {
	skipIfNoCredentials(t)
	_, stderr, code := run(t, "mail", "messages", "trash")
	if code == 0 {
		t.Error("expected error when no REF and no filter given")
	}
	assertContains(t, stderr, "no messages selected")
}

// ── conversations ──

func TestMailConversationsList(t *testing.T) {
	skipIfNoCredentials(t)
	stdout := runOK(t, "mail", "conversations", "list", "--page-size", "5")
	assertContains(t, stdout, "SUBJECT")
}

func TestMailConversationsListJSONShape(t *testing.T) {
	skipIfNoCredentials(t)
	data := runJSON(t, "mail", "conversations", "list", "--page-size", "3")
	if _, ok := data["total"]; !ok {
		t.Error("expected 'total' key")
	}
	convs, ok := data["conversations"].([]interface{})
	if !ok {
		t.Fatal("expected 'conversations' array")
	}
	if len(convs) > 0 {
		c := convs[0].(map[string]interface{})
		for _, field := range []string{"id", "subject", "num_messages", "time"} {
			if _, has := c[field]; !has {
				t.Errorf("expected snake_case field %q, got keys: %v", field, keysOf(c))
			}
		}
	}
}

// findConversationFor returns the conversation ID containing the given
// message ID by scanning the inbox via list. Skips on miss.
func findConversationFor(t *testing.T, msgID string) string {
	t.Helper()
	// /mail/v4/conversations?ID=<msgID> doesn't work; fall back to fetching
	// the message via the api escape hatch and reading ConversationID.
	stdout := runOK(t, "api", "GET", "/mail/v4/messages/"+msgID)
	var v map[string]interface{}
	if err := json.Unmarshal([]byte(stdout), &v); err != nil {
		t.Fatalf("api GET message: %v", err)
	}
	m, _ := v["Message"].(map[string]interface{})
	if m == nil {
		t.Skip("could not extract message envelope from api response")
	}
	convID, _ := m["ConversationID"].(string)
	if convID == "" {
		t.Skip("message has no ConversationID")
	}
	return convID
}

func TestMailConversationsRead(t *testing.T) {
	skipIfNoCredentials(t)
	subject := testID() + "-conv-read"
	msgID := sendTestMail(t, subject)
	convID := findConversationFor(t, msgID)

	stdout := runOK(t, "mail", "conversations", "read", "--", convID)
	assertContains(t, stdout, subject)
	assertContains(t, stdout, "Subject:")
	assertContains(t, stdout, "Conversation:")
	assertContains(t, stdout, "Messages:")
}

func TestMailMessagesReadConvIDRedirects(t *testing.T) {
	skipIfNoCredentials(t)
	subject := testID() + "-redir-msg"
	msgID := sendTestMail(t, subject)
	convID := findConversationFor(t, msgID)

	_, stderr, code := run(t, "mail", "messages", "read", "--", convID)
	if code != 3 {
		t.Errorf("expected exit 3, got %d (stderr: %s)", code, stderr)
	}
	assertContains(t, stderr, "that ID is a conversation, not a message")
	assertContains(t, stderr, "proton-cli mail conversations read")
	assertContains(t, stderr, convID)
}

func TestMailConversationsReadMsgIDRedirects(t *testing.T) {
	skipIfNoCredentials(t)
	subject := testID() + "-redir-conv"
	msgID := sendTestMail(t, subject)

	_, stderr, code := run(t, "mail", "conversations", "read", "--", msgID)
	if code != 3 {
		t.Errorf("expected exit 3, got %d (stderr: %s)", code, stderr)
	}
	assertContains(t, stderr, "that ID is a message, not a conversation")
	assertContains(t, stderr, "proton-cli mail messages read")
	assertContains(t, stderr, msgID)
}

func TestMailConversationsBulkDryRun(t *testing.T) {
	skipIfNoCredentials(t)
	_, stderr := runOKStderr(t, "--dry-run", "mail", "conversations", "trash",
		"--unread", "--limit", "3")
	assertContains(t, stderr, "dry-run")
}

func TestMailConversationsTrashRoundTrip(t *testing.T) {
	skipIfNoCredentials(t)
	subject := testID() + "-conv-trash"
	msgID := sendTestMail(t, subject)
	convID := findConversationFor(t, msgID)

	runOK(t, "mail", "conversations", "trash", "--", convID)
	data := runJSON(t, "mail", "conversations", "list", "--page-size", "50")
	convs := data["conversations"].([]interface{})
	for _, c := range convs {
		if c.(map[string]interface{})["id"].(string) == convID {
			t.Error("trashed conversation should not appear in inbox list")
		}
	}
	runOK(t, "mail", "conversations", "move", "--dest", "inbox", "--", convID)
}

// ── attachments ──

func TestMailAttachmentsListAndDownload(t *testing.T) {
	skipIfNoCredentials(t)

	// Find any message with an attachment.
	data := runJSON(t, "mail", "messages", "list", "--page-size", "50")
	msgs := data["messages"].([]interface{})
	var msgID string
	for _, m := range msgs {
		msg := m.(map[string]interface{})
		if num, ok := msg["num_attachments"].(float64); ok && num > 0 {
			msgID = msg["id"].(string)
			break
		}
	}
	if msgID == "" {
		t.Skip("no messages with attachments found in last 50 inbox items")
	}

	// List
	stdout := runOK(t, "mail", "attachments", "list", msgID)
	assertContains(t, stdout, "NAME")

	// Download to tempdir
	attsRaw := runOK(t, "mail", "attachments", "list", msgID, "--output", "json")
	var atts []map[string]interface{}
	if err := json.Unmarshal([]byte(attsRaw), &atts); err != nil {
		t.Fatalf("failed to parse attachments JSON: %v", err)
	}
	if len(atts) == 0 {
		t.Skip("no attachments after decryption")
	}
	attID := atts[0]["id"].(string)
	attName, _ := atts[0]["name"].(string)
	out := filepath.Join(t.TempDir(), "att")
	runOK(t, "mail", "attachments", "download", msgID, attID, out)

	info, err := os.Stat(out)
	if err != nil {
		t.Fatalf("attachment not saved: %v", err)
	}
	if info.Size() == 0 {
		t.Errorf("attachment %q is empty", attName)
	}
}

// findMessageWithAttachment scans the inbox for a message with at least one
// attachment and returns (msgID, attID, attName). Skips the test on miss.
func findMessageWithAttachment(t *testing.T) (msgID, attID, attName string) {
	t.Helper()
	data := runJSON(t, "mail", "messages", "list", "--page-size", "50")
	msgs := data["messages"].([]interface{})
	for _, m := range msgs {
		mm := m.(map[string]interface{})
		if n, ok := mm["num_attachments"].(float64); ok && n > 0 {
			msgID = mm["id"].(string)
			break
		}
	}
	if msgID == "" {
		t.Skip("no messages with attachments in last 50 inbox items")
	}
	attsRaw := runOK(t, "mail", "attachments", "list", msgID, "--output", "json")
	var atts []map[string]interface{}
	if err := json.Unmarshal([]byte(attsRaw), &atts); err != nil {
		t.Fatalf("parse attachments JSON: %v", err)
	}
	if len(atts) == 0 {
		t.Skip("no attachments after decryption")
	}
	attID = atts[0]["id"].(string)
	attName, _ = atts[0]["name"].(string)
	return msgID, attID, attName
}

func TestMailAttachmentsDownloadCollisionAutoSuffix(t *testing.T) {
	skipIfNoCredentials(t)
	msgID, attID, attName := findMessageWithAttachment(t)

	dir := t.TempDir()
	// Pre-create the canonical destination so the download collides.
	placeholder := filepath.Join(dir, attName)
	if err := os.WriteFile(placeholder, []byte("placeholder"), 0644); err != nil {
		t.Fatalf("setup: %v", err)
	}

	_, stderr := runOKStderr(t, "mail", "attachments", "download", msgID, attID,
		"--output-dir", dir)

	// Suffixed file must exist; placeholder must be untouched.
	stem := strings.TrimSuffix(attName, filepath.Ext(attName))
	ext := filepath.Ext(attName)
	suffixed := filepath.Join(dir, stem+"_1"+ext)
	if _, err := os.Stat(suffixed); err != nil {
		t.Errorf("expected auto-suffixed file at %s, got: %v\nstderr: %s", suffixed, err, stderr)
	}
	if data, _ := os.ReadFile(placeholder); string(data) != "placeholder" {
		t.Errorf("placeholder was overwritten: %q", string(data))
	}
}

func TestMailAttachmentsDownloadCollisionExplicitErrors(t *testing.T) {
	skipIfNoCredentials(t)
	msgID, attID, _ := findMessageWithAttachment(t)

	dest := filepath.Join(t.TempDir(), "existing.bin")
	if err := os.WriteFile(dest, []byte("x"), 0644); err != nil {
		t.Fatalf("setup: %v", err)
	}
	_, stderr, code := run(t, "mail", "attachments", "download", msgID, attID,
		"--output", dest)
	if code == 0 {
		t.Errorf("expected non-zero exit on collision, got 0")
	}
	if !strings.Contains(stderr, "exists") {
		t.Errorf("expected stderr to mention 'exists', got: %s", stderr)
	}
}

func TestMailAttachmentsDownloadForce(t *testing.T) {
	skipIfNoCredentials(t)
	msgID, attID, _ := findMessageWithAttachment(t)

	dest := filepath.Join(t.TempDir(), "existing.bin")
	if err := os.WriteFile(dest, []byte("placeholder"), 0644); err != nil {
		t.Fatalf("setup: %v", err)
	}
	runOK(t, "mail", "attachments", "download", msgID, attID,
		"--output", dest, "--force")
	data, err := os.ReadFile(dest)
	if err != nil {
		t.Fatalf("read after force: %v", err)
	}
	if string(data) == "placeholder" {
		t.Error("--force did not overwrite")
	}
}

func TestMailAttachmentsDownloadAll(t *testing.T) {
	skipIfNoCredentials(t)
	msgID, _, _ := findMessageWithAttachment(t)

	dir := t.TempDir()
	runOK(t, "mail", "attachments", "download", msgID, "--all", "--output-dir", dir)

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("readdir: %v", err)
	}
	if len(entries) == 0 {
		t.Error("--all wrote no files")
	}
}

func TestMailAttachmentsDownloadAllRequiresDir(t *testing.T) {
	skipIfNoCredentials(t)
	_, stderr, code := run(t, "mail", "attachments", "download", "any-msg-id", "--all")
	if code == 0 {
		t.Error("expected non-zero exit when --all is missing --output-dir")
	}
	if !strings.Contains(stderr, "--output-dir") {
		t.Errorf("expected stderr to mention --output-dir, got: %s", stderr)
	}
}

func TestMailAttachmentsDownloadAllRejectsStdout(t *testing.T) {
	skipIfNoCredentials(t)
	_, stderr, code := run(t, "mail", "attachments", "download", "any-msg-id",
		"--all", "--output", "-")
	if code == 0 {
		t.Error("expected non-zero exit for --all --output -")
	}
	if !strings.Contains(stderr, "stdout") {
		t.Errorf("expected stderr to mention stdout, got: %s", stderr)
	}
}

func TestMailAttachmentsDownloadOutputDirAutoCreates(t *testing.T) {
	skipIfNoCredentials(t)
	msgID, attID, _ := findMessageWithAttachment(t)

	dir := filepath.Join(t.TempDir(), "new", "deep", "nested")
	runOK(t, "mail", "attachments", "download", msgID, attID, "--output-dir", dir)

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("--output-dir was not created: %v", err)
	}
	if len(entries) == 0 {
		t.Error("no file written into auto-created --output-dir")
	}
}

// ── inline-disposition filter (#06) ──

// findMessageWithMixedAttachments scans the inbox for a message that has at
// least one inline AND one non-inline attachment. Skips the test if no such
// message exists in the last 50 inbox items.
func findMessageWithMixedAttachments(t *testing.T) (msgID string) {
	t.Helper()
	data := runJSON(t, "mail", "messages", "list", "--page-size", "50")
	msgs := data["messages"].([]interface{})
	for _, m := range msgs {
		mm := m.(map[string]interface{})
		n, _ := mm["num_attachments"].(float64)
		if n < 2 {
			continue
		}
		id := mm["id"].(string)
		raw := runOK(t, "mail", "attachments", "list", id,
			"--include-inline", "--output", "json")
		var atts []map[string]interface{}
		if err := json.Unmarshal([]byte(raw), &atts); err != nil {
			continue
		}
		hasInline, hasReal := false, false
		for _, a := range atts {
			switch a["disposition"].(string) {
			case "inline":
				hasInline = true
			default:
				hasReal = true
			}
		}
		if hasInline && hasReal {
			return id
		}
	}
	t.Skip("no message with mixed inline/attachment dispositions found")
	return ""
}

func TestMailAttachmentsListFiltersInline(t *testing.T) {
	skipIfNoCredentials(t)
	msgID := findMessageWithMixedAttachments(t)

	// Default: filtered. Text mode must NOT have a DISPOSITION header.
	stdout := runOK(t, "mail", "attachments", "list", msgID)
	if strings.Contains(stdout, "DISPOSITION") {
		t.Error("default text-mode list should not show DISPOSITION column")
	}

	// JSON: filtered, but each entry still has the disposition field.
	defaultRaw := runOK(t, "mail", "attachments", "list", msgID, "--output", "json")
	var defaultAtts []map[string]interface{}
	if err := json.Unmarshal([]byte(defaultRaw), &defaultAtts); err != nil {
		t.Fatalf("parse default JSON: %v", err)
	}
	for _, a := range defaultAtts {
		if d, _ := a["disposition"].(string); d == "inline" {
			t.Errorf("default list contains inline attachment: %v", a)
		}
		if _, ok := a["disposition"]; !ok {
			t.Errorf("default JSON missing disposition field on %v", a)
		}
	}

	// --include-inline: text mode shows DISPOSITION column.
	stdoutAll := runOK(t, "mail", "attachments", "list", msgID, "--include-inline")
	if !strings.Contains(stdoutAll, "DISPOSITION") {
		t.Error("--include-inline text-mode list should show DISPOSITION column")
	}

	// --include-inline JSON has both kinds.
	allRaw := runOK(t, "mail", "attachments", "list", msgID,
		"--include-inline", "--output", "json")
	var allAtts []map[string]interface{}
	if err := json.Unmarshal([]byte(allRaw), &allAtts); err != nil {
		t.Fatalf("parse --include-inline JSON: %v", err)
	}
	dispositions := map[string]bool{}
	for _, a := range allAtts {
		if d, ok := a["disposition"].(string); ok {
			dispositions[d] = true
		}
	}
	if !dispositions["inline"] {
		t.Error("--include-inline list should include at least one inline")
	}
	if !dispositions["attachment"] {
		t.Error("--include-inline list should include at least one attachment")
	}
	if len(allAtts) <= len(defaultAtts) {
		t.Errorf("--include-inline (%d) should yield more entries than default (%d)",
			len(allAtts), len(defaultAtts))
	}
}

func TestMailAttachmentsListJSONHasDisposition(t *testing.T) {
	skipIfNoCredentials(t)
	msgID, _, _ := findMessageWithAttachment(t)

	raw := runOK(t, "mail", "attachments", "list", msgID, "--output", "json")
	var atts []map[string]interface{}
	if err := json.Unmarshal([]byte(raw), &atts); err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(atts) == 0 {
		t.Skip("no attachments after default filter")
	}
	for _, a := range atts {
		d, ok := a["disposition"].(string)
		if !ok {
			t.Errorf("attachment missing 'disposition' field: %v", a)
		}
		// May be "" on legacy messages; that's still a string.
		_ = d
	}
}

func TestMailAttachmentsDownloadAllSkipsInline(t *testing.T) {
	skipIfNoCredentials(t)
	msgID := findMessageWithMixedAttachments(t)

	dir := t.TempDir()
	runOK(t, "mail", "attachments", "download", msgID, "--all", "--output-dir", dir)

	written, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("readdir: %v", err)
	}
	numWritten := len(written)

	// --include-inline should yield strictly more files.
	dir2 := t.TempDir()
	runOK(t, "mail", "attachments", "download", msgID,
		"--all", "--include-inline", "--output-dir", dir2)
	written2, err := os.ReadDir(dir2)
	if err != nil {
		t.Fatalf("readdir2: %v", err)
	}
	if len(written2) <= numWritten {
		t.Errorf("--include-inline (%d files) should write more than default (%d files)",
			len(written2), numWritten)
	}
}

// ── strip-quotes / summary (#10) ──

func TestMailMessagesReadStripQuotesPlaintext(t *testing.T) {
	skipIfNoCredentials(t)
	// Self-mail with the canonical "On X, <a@b> wrote:" + "> ..." reply
	// shape baked into the body. sendTestMail uses --body, plain text by
	// default. We can't easily craft this through the CLI because the body
	// is one --body argument and the canonical pattern needs newlines.
	subject := testID() + "-strip-quotes"
	body := "My new note.\n\nOn Tue, 24 Sep 2024, Sender <a@b.com> wrote:\n\n> ancient quoted text\n> that should disappear\n"
	runOK(t, "mail", "messages", "send",
		"--to", selfEmail(), "--subject", subject, "--body", body)

	// Find the inbox copy.
	var msgID string
	for attempt := 0; attempt < 10 && msgID == ""; attempt++ {
		time.Sleep(2 * time.Second)
		data := runJSON(t, "mail", "messages", "list", "--folder", "inbox", "--page-size", "20")
		for _, m := range data["messages"].([]interface{}) {
			mm := m.(map[string]interface{})
			if s, _ := mm["subject"].(string); s == subject {
				msgID, _ = mm["id"].(string)
				break
			}
		}
	}
	if msgID == "" {
		t.Skip("self-mail did not arrive in inbox in time")
	}
	cleanupRun(t, "Delete inbox: proton-cli mail messages delete -- "+msgID,
		"mail", "messages", "delete", "--", msgID)

	default1 := runOK(t, "mail", "messages", "read", msgID)
	if !strings.Contains(default1, "ancient quoted text") {
		t.Errorf("default mode should preserve the quote; stdout:\n%s", truncateOutput(default1))
	}

	stripped := runOK(t, "mail", "messages", "read", "--strip-quotes", msgID)
	if strings.Contains(stripped, "ancient quoted text") {
		t.Errorf("--strip-quotes should remove the quote; stdout:\n%s", truncateOutput(stripped))
	}
	if !strings.Contains(stripped, "My new note") {
		t.Errorf("--strip-quotes should preserve new content; stdout:\n%s", truncateOutput(stripped))
	}
}

func TestMailMessagesReadStripQuotesNoFalsePositive(t *testing.T) {
	skipIfNoCredentials(t)
	subject := testID() + "-strip-no-quote"
	msgID := sendTestMail(t, subject)

	default1 := runOK(t, "mail", "messages", "read", msgID)
	stripped := runOK(t, "mail", "messages", "read", "--strip-quotes", msgID)
	// On a body with no canonical reply marker, --strip-quotes is a no-op.
	if default1 != stripped {
		t.Errorf("--strip-quotes should be a no-op on bodies without quote markers")
	}
}

func TestMailConversationsReadSummary(t *testing.T) {
	skipIfNoCredentials(t)
	subject := testID() + "-summary"
	msgID := sendTestMail(t, subject)
	convID := findConversationFor(t, msgID)

	data := runJSON(t, "--full-ids", "mail", "conversations", "read", convID)
	// Use the JSON shape to determine the expected message count.
	msgs := data["messages"].([]interface{})
	wantCount := len(msgs)
	if wantCount == 0 {
		t.Skip("conversation has 0 messages")
	}

	stdout := runOK(t, "mail", "conversations", "read", "--summary", convID)
	lines := strings.Split(strings.TrimRight(stdout, "\n"), "\n")
	if len(lines) != wantCount {
		t.Errorf("expected %d summary lines, got %d:\n%s", wantCount, len(lines), stdout)
	}

	// Each line must match: <N>/<M>  <YYYY-MM-DD HH:MM>  <addr>  <preview>
	re := regexp.MustCompile(`^\d+/\d+\s+\d{4}-\d{2}-\d{2} \d{2}:\d{2}\s+\S+@\S+\s+`)
	for i, line := range lines {
		if !re.MatchString(line) {
			t.Errorf("line %d does not match summary shape: %q", i, line)
		}
	}
}

func TestMailConversationsReadSummaryAttachmentTag(t *testing.T) {
	skipIfNoCredentials(t)
	msgID, _, _ := findMessageWithAttachment(t)
	convID := findConversationFor(t, msgID)
	stdout := runOK(t, "mail", "conversations", "read", "--summary", convID)
	// Some line must end with `(N attachments)`.
	re := regexp.MustCompile(`\(\d+ attachments\)\s*$`)
	found := false
	for _, line := range strings.Split(strings.TrimRight(stdout, "\n"), "\n") {
		if re.MatchString(line) {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected at least one summary line ending with `(N attachments)`:\n%s", truncateOutput(stdout))
	}
}

func TestMailConversationsReadStripQuotesKeepsLayout(t *testing.T) {
	skipIfNoCredentials(t)
	subject := testID() + "-conv-strip"
	msgID := sendTestMail(t, subject)
	convID := findConversationFor(t, msgID)

	stdout := runOK(t, "mail", "conversations", "read", "--strip-quotes", convID)
	// Layout should still have per-message dividers + headers.
	if !strings.Contains(stdout, "\u2500\u2500\u2500 1/") {
		t.Errorf("--strip-quotes (without --summary) should keep per-message dividers; stdout:\n%s", truncateOutput(stdout))
	}
	if !strings.Contains(stdout, "From: ") {
		t.Errorf("--strip-quotes should keep per-message headers; stdout:\n%s", truncateOutput(stdout))
	}
}

// ── messages/conversations read --body-only (#08) ──

func TestMailMessagesReadBodyOnly(t *testing.T) {
	skipIfNoCredentials(t)
	subject := testID() + "-body-only"
	msgID := sendTestMail(t, subject)
	stdout := runOK(t, "mail", "messages", "read", "--body-only", msgID)
	for _, marker := range []string{"Subject:", "From:", "To:", "ID:", "---", "Attachments ("} {
		if strings.Contains(stdout, marker) {
			t.Errorf("--body-only output should not contain %q; got:\n%s", marker, truncateOutput(stdout))
		}
	}
	// The body itself contains the subject (sendTestMail's body template).
	if !strings.Contains(stdout, subject) {
		t.Errorf("--body-only stripped the body too aggressively; subject %q missing", subject)
	}
}

func TestMailMessagesReadFormatHTMLNoHeader(t *testing.T) {
	skipIfNoCredentials(t)
	subject := testID() + "-fmt-html"
	msgID := sendTestMail(t, subject)
	stdout := runOK(t, "mail", "messages", "read", "--format", "html", msgID)
	if strings.HasPrefix(strings.TrimSpace(stdout), "Subject:") {
		t.Errorf("--format html must not start with 'Subject:' header; got:\n%s", truncateOutput(stdout))
	}
	for _, marker := range []string{"\nSubject: ", "\nFrom:    ", "\nTo:      ", "\nID:      "} {
		if strings.Contains(stdout, marker) {
			t.Errorf("--format html output should not contain header marker %q", marker)
		}
	}
}

func TestMailMessagesReadFormatRawNoHeader(t *testing.T) {
	skipIfNoCredentials(t)
	subject := testID() + "-fmt-raw"
	msgID := sendTestMail(t, subject)
	stdout := runOK(t, "mail", "messages", "read", "--format", "raw", msgID)
	if strings.HasPrefix(strings.TrimSpace(stdout), "Subject:") {
		t.Errorf("--format raw must not start with 'Subject:' header; got:\n%s", truncateOutput(stdout))
	}
}

func TestMailMessagesReadDefaultStillHasHeader(t *testing.T) {
	skipIfNoCredentials(t)
	subject := testID() + "-default"
	msgID := sendTestMail(t, subject)
	stdout := runOK(t, "mail", "messages", "read", msgID)
	assertContains(t, stdout, "Subject:")
	assertContains(t, stdout, "From:")
}

func TestMailConversationsReadBodyOnly(t *testing.T) {
	skipIfNoCredentials(t)
	subject := testID() + "-conv-body"
	msgID := sendTestMail(t, subject)
	convID := findConversationFor(t, msgID)

	stdout := runOK(t, "mail", "conversations", "read", "--body-only", convID)
	for _, marker := range []string{
		"Subject:      ",
		"Conversation: ",
		"Messages:     ",
		"─── 1/",
		"\nFrom: ",
		"\nDate: ",
		"---",
		"Attachments (",
	} {
		if strings.Contains(stdout, marker) {
			t.Errorf("--body-only conv read should not contain %q; got:\n%s", marker, truncateOutput(stdout))
		}
	}
	// At least one body should reach stdout (this self-mail's body).
	if !strings.Contains(stdout, subject) {
		t.Errorf("--body-only conv read missing body containing %q", subject)
	}
}

// ── messages read attachments footer (#07) ──

func TestMailMessagesReadShowsAttachments(t *testing.T) {
	skipIfNoCredentials(t)
	msgID, _, _ := findMessageWithAttachment(t)
	stdout := runOK(t, "mail", "messages", "read", msgID)
	if !strings.Contains(stdout, "\n---\n") {
		t.Errorf("expected '---' separator before footer in:\n%s", truncateOutput(stdout))
	}
	if !strings.Contains(stdout, "Attachments (") {
		t.Errorf("expected 'Attachments (N):' line, got:\n%s", truncateOutput(stdout))
	}
}

func TestMailMessagesReadNoAttachmentsNoFooter(t *testing.T) {
	skipIfNoCredentials(t)
	subject := testID() + "-no-atts"
	msgID := sendTestMail(t, subject)
	stdout := runOK(t, "mail", "messages", "read", msgID)
	if strings.Contains(stdout, "---") {
		t.Errorf("unexpected '---' separator on no-attachments message:\n%s", truncateOutput(stdout))
	}
	if strings.Contains(stdout, "Attachments (") {
		t.Errorf("unexpected attachments footer on no-attachments message:\n%s", truncateOutput(stdout))
	}
}

func TestMailMessagesReadFormatHTMLNoFooter(t *testing.T) {
	skipIfNoCredentials(t)
	msgID, _, _ := findMessageWithAttachment(t)
	stdout := runOK(t, "mail", "messages", "read", "--format", "html", msgID)
	if strings.Contains(stdout, "Attachments (") {
		t.Errorf("--format html must not append the footer:\n%s", truncateOutput(stdout))
	}
}

func TestMailMessagesReadFormatRawNoFooter(t *testing.T) {
	skipIfNoCredentials(t)
	msgID, _, _ := findMessageWithAttachment(t)
	stdout := runOK(t, "mail", "messages", "read", "--format", "raw", msgID)
	if strings.Contains(stdout, "Attachments (") {
		t.Errorf("--format raw must not append the footer:\n%s", truncateOutput(stdout))
	}
}

func TestMailMessagesReadIncludeInlineTags(t *testing.T) {
	skipIfNoCredentials(t)
	msgID := findMessageWithMixedAttachments(t)

	default1 := runOK(t, "mail", "messages", "read", msgID)
	if strings.Contains(default1, "(inline)") {
		t.Errorf("default footer should not show (inline) markers:\n%s", truncateOutput(default1))
	}

	incl := runOK(t, "mail", "messages", "read", "--include-inline", msgID)
	if !strings.Contains(incl, "(inline)") {
		t.Errorf("--include-inline footer should show at least one (inline) marker:\n%s", truncateOutput(incl))
	}
}

func TestMailConversationsReadShowsAttachmentsPerMessage(t *testing.T) {
	skipIfNoCredentials(t)
	msgID, _, _ := findMessageWithAttachment(t)
	convID := findConversationFor(t, msgID)
	stdout := runOK(t, "mail", "conversations", "read", convID)
	if !strings.Contains(stdout, "Attachments (") {
		t.Errorf("conversation render should contain a per-message attachments footer:\n%s", truncateOutput(stdout))
	}
}

func TestMailConversationsReadIncludeInline(t *testing.T) {
	skipIfNoCredentials(t)
	msgID := findMessageWithMixedAttachments(t)
	convID := findConversationFor(t, msgID)

	default1 := runOK(t, "mail", "conversations", "read", convID)
	if strings.Contains(default1, "(inline)") {
		t.Errorf("default conv read should not include (inline):\n%s", truncateOutput(default1))
	}

	incl := runOK(t, "mail", "conversations", "read", "--include-inline", convID)
	if !strings.Contains(incl, "(inline)") {
		t.Errorf("--include-inline conv read should include (inline):\n%s", truncateOutput(incl))
	}
}

// ── mail conversations attachments ──

func TestMailConversationsAttachmentsList(t *testing.T) {
	skipIfNoCredentials(t)
	msgID, _, _ := findMessageWithAttachment(t)
	convID := findConversationFor(t, msgID)

	// Default: filtered, includes a MESSAGE_ID column.
	stdout := runOK(t, "mail", "conversations", "attachments", "list", convID)
	assertContains(t, stdout, "MESSAGE_ID")
	if strings.Contains(stdout, "DISPOSITION") {
		t.Error("default text-mode list must not show DISPOSITION column")
	}

	// JSON: each entry carries message_id + disposition.
	raw := runOK(t, "--output", "json", "mail", "conversations", "attachments", "list", convID)
	var atts []map[string]interface{}
	if err := json.Unmarshal([]byte(raw), &atts); err != nil {
		t.Fatalf("parse JSON: %v", err)
	}
	if len(atts) == 0 {
		t.Skip("no attachments after default filter")
	}
	for _, a := range atts {
		if _, ok := a["message_id"].(string); !ok {
			t.Errorf("attachment missing message_id: %v", a)
		}
		if _, ok := a["disposition"]; !ok {
			t.Errorf("attachment missing disposition: %v", a)
		}
	}
}

func TestMailConversationsAttachmentsListIncludeInline(t *testing.T) {
	skipIfNoCredentials(t)
	msgID := findMessageWithMixedAttachments(t)
	convID := findConversationFor(t, msgID)

	stdout := runOK(t, "mail", "conversations", "attachments", "list",
		"--include-inline", convID)
	assertContains(t, stdout, "DISPOSITION")
	assertContains(t, stdout, "inline")
}

func TestMailConversationsAttachmentsDownloadAll(t *testing.T) {
	skipIfNoCredentials(t)
	msgID, _, _ := findMessageWithAttachment(t)
	convID := findConversationFor(t, msgID)

	dir := t.TempDir()
	runOK(t, "mail", "conversations", "attachments", "download", convID,
		"--all", "--output-dir", dir)

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("readdir: %v", err)
	}
	if len(entries) == 0 {
		t.Error("--all wrote no files")
	}
}

func TestMailConversationsAttachmentsDownloadAllSkipsInline(t *testing.T) {
	skipIfNoCredentials(t)
	msgID := findMessageWithMixedAttachments(t)
	convID := findConversationFor(t, msgID)

	dir := t.TempDir()
	runOK(t, "mail", "conversations", "attachments", "download", convID,
		"--all", "--output-dir", dir)
	default1, _ := os.ReadDir(dir)

	dir2 := t.TempDir()
	runOK(t, "mail", "conversations", "attachments", "download", convID,
		"--all", "--include-inline", "--output-dir", dir2)
	inclAll, _ := os.ReadDir(dir2)

	if len(inclAll) <= len(default1) {
		t.Errorf("--include-inline (%d files) should write more than default (%d files)",
			len(inclAll), len(default1))
	}
}

func TestMailConversationsAttachmentsDownloadOneByID(t *testing.T) {
	skipIfNoCredentials(t)
	msgID, attID, attName := findMessageWithAttachment(t)
	convID := findConversationFor(t, msgID)

	dir := t.TempDir()
	runOK(t, "mail", "conversations", "attachments", "download", convID, attID,
		"--output-dir", dir)

	dest := filepath.Join(dir, attName)
	info, err := os.Stat(dest)
	if err != nil {
		t.Fatalf("single download not at %s: %v", dest, err)
	}
	if info.Size() == 0 {
		t.Errorf("downloaded file is empty")
	}
}

func TestMailConversationsAttachmentsDownloadUnknownID(t *testing.T) {
	skipIfNoCredentials(t)
	msgID, _, _ := findMessageWithAttachment(t)
	convID := findConversationFor(t, msgID)

	_, stderr, code := run(t, "mail", "conversations", "attachments", "download",
		convID, "fake-attachment-id-"+testID())
	if code != 3 {
		t.Errorf("expected exit 3 for unknown attachment, got %d", code)
	}
	if !strings.Contains(stderr, "not found in conversation") {
		t.Errorf("expected 'not found in conversation' in stderr, got: %s", stderr)
	}
}

// ── labels ──

func TestMailLabelsList(t *testing.T) {
	skipIfNoCredentials(t)
	stdout := runOK(t, "mail", "labels", "list")
	assertContains(t, stdout, "NAME")
}

func TestMailLabelsCreateDeleteLabel(t *testing.T) {
	skipIfNoCredentials(t)
	name := testID() + "-label"

	// stdout = just the ID
	stdout := runOK(t, "mail", "labels", "create", "--name", name, "--color", "#8080FF")
	id := strings.TrimSpace(stdout)
	if !looksLikeID(id) {
		t.Fatalf("expected bare ID on stdout, got %q", stdout)
	}
	cleanupRun(t, fmt.Sprintf("Delete label: proton-cli mail labels delete -- %s", id),
		"mail", "labels", "delete", "--", id)

	list := runOK(t, "mail", "labels", "list")
	assertContains(t, list, name)
	assertContains(t, list, "LABEL")
}

func TestMailLabelsCreateFolder(t *testing.T) {
	skipIfNoCredentials(t)
	name := testID() + "-folder"
	stdout := runOK(t, "mail", "labels", "create", "--name", name, "--folder", "--color", "#8080FF")
	id := strings.TrimSpace(stdout)
	cleanupRun(t, fmt.Sprintf("Delete folder: proton-cli mail labels delete -- %s", id),
		"mail", "labels", "delete", "--", id)
	list := runOK(t, "mail", "labels", "list")
	assertContains(t, list, name)
	assertContains(t, list, "FOLDER")
}

// ── filters ──

func TestMailFiltersCRUD(t *testing.T) {
	skipIfNoCredentials(t)
	name := testID() + "-filter"
	sieve := `require ["fileinto"]; if header :contains "Subject" "xyz-never-matches-` + testID() + `" { fileinto "Archive"; }`

	stdout := runOK(t, "mail", "filters", "create", "--name", name, "--sieve", sieve)
	id := strings.TrimSpace(stdout)
	if !looksLikeID(id) {
		t.Fatalf("expected bare ID on stdout, got %q", stdout)
	}
	cleanupRun(t, fmt.Sprintf("Delete filter: proton-cli mail filters delete -- %s", id),
		"mail", "filters", "delete", "--", id)

	list := runOK(t, "mail", "filters", "list")
	assertContains(t, list, name)
	assertContains(t, list, "enabled")

	runOK(t, "mail", "filters", "disable", "--", id)
	assertContains(t, runOK(t, "mail", "filters", "list"), "disabled")

	runOK(t, "mail", "filters", "enable", "--", id)
	assertContains(t, runOK(t, "mail", "filters", "list"), "enabled")
}

// ── addresses ──

func TestMailAddressesList(t *testing.T) {
	skipIfNoCredentials(t)
	stdout := runOK(t, "mail", "addresses", "list")
	assertContains(t, stdout, "EMAIL")
	assertContains(t, stdout, selfEmail())
}

// ── helpers local to mail tests ──

func keysOf(m map[string]interface{}) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

// looksLikeID matches a Proton base64 ID (~88 chars ending in ==).
func looksLikeID(s string) bool {
	return len(s) > 60 && strings.HasSuffix(s, "==")
}
