package tests

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"
)

// ── mail messages list ──

func TestMailMessagesList(t *testing.T) {
	t.Parallel()
	stdout := runOK(t, "mail", "messages", "list")
	assertContains(t, stdout, "ID")
	assertContains(t, stdout, "FROM")
	assertContains(t, stdout, "SUBJECT")
}

func TestMailMessagesListSent(t *testing.T) {
	t.Parallel()
	runOK(t, "mail", "messages", "list", "--folder", "sent")
}

func TestMailMessagesListJSONFieldNames(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
	data := runJSON(t, "mail", "messages", "list", "--page-size", "3")
	msgs := data["messages"].([]interface{})
	if len(msgs) > 3 {
		t.Errorf("expected at most 3 messages, got %d", len(msgs))
	}
}

func TestMailMessagesListUnreadFlag(t *testing.T) {
	t.Parallel()
	runOK(t, "mail", "messages", "list", "--unread")
}

// ── list footer / json shape ──

func TestMailMessagesListFooterSinglePage(t *testing.T) {
	t.Parallel()
	// Use 150 (Proton's documented max for messages list); 500 is
	// rejected as "Invalid page size parameter".
	_, stderr := runOKStderr(t, "mail", "messages", "list", "--page-size", "150")
	last := lastNonEmpty(stderr)
	// One page holds everything, so the footer is a plain count: no "of", no
	// next-page instruction, and never a page number the reader did not ask for.
	if !strings.HasSuffix(last, "messages.") && !strings.HasSuffix(last, "message.") {
		t.Errorf("expected a plain count footer, got: %q", last)
	}
	if strings.Contains(last, "--page") || strings.Contains(last, " of ") {
		t.Errorf("a single page should not offer another: %q", last)
	}
}

func TestMailMessagesListFooterMidPagination(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
	data := runJSON(t, "mail", "messages", "list", "--page-size", "1")
	for _, key := range []string{"total", "page", "page_size", "has_more", "messages"} {
		if _, ok := data[key]; !ok {
			t.Errorf("expected JSON field %q, got keys: %v", key, keysOf(data))
		}
	}
}

func TestMailMessagesSearchFooterNoPageZero(t *testing.T) {
	t.Parallel()
	_, stderr := runOKStderr(t, "mail", "messages", "search", "--keyword", "proton", "--limit", "5")
	last := lastNonEmpty(stderr)
	if strings.Contains(last, "page 0") {
		t.Errorf("search footer still has '(page 0)': %q", last)
	}
	// Either the limit was reached and more may exist, or it was not and the
	// count stands on its own.
	if !strings.HasSuffix(last, "messages.") && !strings.HasSuffix(last, "raise --limit.") {
		t.Errorf("expected a search footer, got: %q", last)
	}
}

func TestMailMessagesSearchEmptyFooter(t *testing.T) {
	t.Parallel()
	_, stderr := runOKStderr(t, "mail", "messages", "search",
		"--keyword", "xyz-no-match-"+testID())
	// A search that matched nothing says so. "No messages." would read as an
	// empty mailbox, which is a different and more alarming fact.
	last := lastNonEmpty(stderr)
	if last != "No messages match." {
		t.Errorf("expected 'No messages match.' on an empty search, got: %q", last)
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
	t.Parallel()
	runOK(t, "mail", "messages", "search", "--keyword", "proton")
}

func TestMailMessagesSearchFrom(t *testing.T) {
	t.Parallel()
	runOK(t, "mail", "messages", "search", "--from", selfEmail())
}

func TestMailMessagesSearchDateRange(t *testing.T) {
	t.Parallel()
	runOK(t, "mail", "messages", "search", "--after", "2020-01-01", "--before", "2099-12-31")
}

func TestMailMessagesSearchEmpty(t *testing.T) {
	t.Parallel()
	_, _, code := run(t, "mail", "messages", "search", "--keyword", "xyz-nothing-xxxyyy-"+testID())
	if code != 0 {
		t.Fatalf("search with no results should exit 0, got %d", code)
	}
}

// ── --from / --to zero-result hint ──

func TestMailSearchFromZeroResultsHint(t *testing.T) {
	t.Parallel()
	needle := "no-such-sender-" + testID()
	_, stderr := runOKStderr(t, "mail", "messages", "search", "--from", needle)
	if !strings.Contains(stderr, "--from matches the address only") {
		t.Errorf("expected the --from hint, got: %s", stderr)
	}
	if !strings.Contains(stderr, "--keyword "+needle) {
		t.Errorf("expected stderr to contain '--keyword %s', got: %s", needle, stderr)
	}
}

func TestMailSearchToZeroResultsHint(t *testing.T) {
	t.Parallel()
	needle := "no-such-rcpt-" + testID()
	_, stderr := runOKStderr(t, "mail", "messages", "search", "--to", needle)
	if !strings.Contains(stderr, "--to matches the address only") {
		t.Errorf("expected the --to hint, got: %s", stderr)
	}
	if !strings.Contains(stderr, "--keyword "+needle) {
		t.Errorf("expected stderr to contain '--keyword %s', got: %s", needle, stderr)
	}
}

func TestMailSearchFromKeywordSuppressesHint(t *testing.T) {
	t.Parallel()
	needle := "impossible-" + testID()
	_, stderr := runOKStderr(t, "mail", "messages", "search",
		"--from", needle, "--keyword", "alsoimpossible-"+testID())
	if strings.Contains(stderr, "matches the address only") {
		t.Errorf("hint should be suppressed when --keyword is set; got: %s", stderr)
	}
}

func TestMailSearchFromHitsNoHint(t *testing.T) {
	t.Parallel()
	plainMail(t) // ensure a delivered self-mail exists and is indexed
	// --from selfEmail() should match. May take a beat to index.
	var stderr string
	for attempt := 0; attempt < 8; attempt++ {
		_, s := runOKStderr(t, "mail", "messages", "search",
			"--from", selfEmail(), "--limit", "5")
		stderr = s
		if !strings.Contains(s, "matches the address only") {
			return
		}
		time.Sleep(2 * time.Second)
	}
	t.Errorf("hint fired on a successful --from query; stderr: %s", stderr)
}

func TestMailSearchFromQuietSuppressesHint(t *testing.T) {
	t.Parallel()
	_, stderr := runOKStderr(t, "--quiet", "mail", "messages", "search",
		"--from", "no-such-sender-"+testID())
	if strings.Contains(stderr, "matches the address only") {
		t.Errorf("--quiet should suppress the hint; got: %s", stderr)
	}
}

func TestMailConversationsSearchFromZeroResultsHint(t *testing.T) {
	t.Parallel()
	needle := "no-such-sender-" + testID()
	_, stderr := runOKStderr(t, "mail", "conversations", "search", "--from", needle)
	if !strings.Contains(stderr, "--from matches the address only") {
		t.Errorf("expected the --from hint, got: %s", stderr)
	}
	if !strings.Contains(stderr, "--keyword "+needle) {
		t.Errorf("expected stderr to contain '--keyword %s', got: %s", needle, stderr)
	}
}

// ── send / read / REF search ──

func TestMailMessagesSendAndReadText(t *testing.T) {
	t.Parallel()
	msgID, _, subject := plainMail(t)

	// Default --format text: human-readable, fields on stderr-safe stdout
	stdout := runOK(t, "mail", "messages", "get", "--", msgID)
	assertContains(t, stdout, subject)
	assertContains(t, stdout, selfEmail())
	assertField(t, stdout, "Subject:", subject)
	// Signature: mail we sent ourselves is signed by our own key.
	assertField(t, stdout, "Signature:", "verified")
}

func TestMailMessagesReadByRef(t *testing.T) {
	t.Parallel()
	_, _, subject := plainMail(t)

	// Proton's search index is populated asynchronously, so the message may
	// show up in list (used by sendTestMail) a few seconds before it shows up
	// in the keyword-search endpoint that REF resolution uses. Retry with
	// backoff instead of hard-failing on the first attempt.
	var stdout, lastStderr string
	var lastCode int
	for attempt := 0; attempt < 8; attempt++ {
		out, stderr, code := run(t, "mail", "messages", "get", subject)
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
	t.Parallel()
	msgID, _, subject := plainMail(t)

	stdout := runOK(t, "mail", "messages", "get", "--format", "raw", "--", msgID)
	assertContains(t, stdout, subject)
}

func TestMailMessagesReadNotFound(t *testing.T) {
	t.Parallel()
	_, _, code := run(t, "mail", "messages", "get", "no-such-msg-"+testID())
	if code != 3 {
		t.Errorf("expected exit 3 (not-found), got %d", code)
	}
}

// ── mark / star / unstar ──

func TestMailMessagesMarkReadUnread(t *testing.T) {
	t.Parallel()
	msgID := mutableMail(t)

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
	t.Parallel()
	msgID := mutableMail(t)

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
	t.Parallel()
	msgID := mutableMail(t)

	runOK(t, "mail", "messages", "move", "--into", "archive", "--", msgID)
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

	runOK(t, "mail", "messages", "move", "--into", "inbox", "--", msgID)
}

func TestMailMessagesTrash(t *testing.T) {
	t.Parallel()
	msgID := mutableMail(t)

	runOK(t, "mail", "messages", "trash", "--", msgID)
	data := runJSON(t, "mail", "messages", "list", "--page-size", "50")
	msgs := data["messages"].([]interface{})
	for _, m := range msgs {
		if m.(map[string]interface{})["id"].(string) == msgID {
			t.Error("trashed message should not appear in inbox")
		}
	}
	// put it back so cleanup can delete
	runOK(t, "mail", "messages", "move", "--into", "inbox", "--", msgID)
}

// ── batch filters (all dry-run so nothing is actually mutated) ──

func TestMailBatchTrashDryRunUnread(t *testing.T) {
	t.Parallel()
	_, stderr := runOKStderr(t, "--dry-run", "mail", "messages", "trash", "--unread", "--limit", "5")
	assertContains(t, stderr, "Dry run")
}

func TestMailBatchTrashDryRunOlderThan(t *testing.T) {
	t.Parallel()
	_, stderr := runOKStderr(t, "--dry-run", "mail", "messages", "trash", "--older-than", "365d", "--from", "noreply", "--limit", "5")
	assertContains(t, stderr, "Dry run")
}

// ── conversations ──

func TestMailConversationsList(t *testing.T) {
	t.Parallel()
	stdout := runOK(t, "mail", "conversations", "list", "--page-size", "5")
	assertContains(t, stdout, "SUBJECT")
}

func TestMailConversationsListJSONShape(t *testing.T) {
	t.Parallel()
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
var (
	convCacheMu sync.Mutex
	convCache   = map[string]string{}
)

func findConversationFor(t *testing.T, msgID string) string {
	t.Helper()
	convCacheMu.Lock()
	cached, ok := convCache[msgID]
	convCacheMu.Unlock()
	if ok {
		return cached
	}
	convID := conversationIDOf(msgID)
	if convID == "" {
		t.Skip("message has no ConversationID")
	}
	convCacheMu.Lock()
	convCache[msgID] = convID
	convCacheMu.Unlock()
	return convID
}

func TestMailConversationsRead(t *testing.T) {
	t.Parallel()
	_, convID, subject := plainMail(t)

	stdout := runOK(t, "mail", "conversations", "get", "--", convID)
	assertContains(t, stdout, subject)
	assertContains(t, stdout, "Subject:")
	assertContains(t, stdout, "Messages:")
	assertContains(t, stdout, "Messages:")
}

func TestMailMessagesReadConvIDRedirects(t *testing.T) {
	t.Parallel()
	_, convID, _ := plainMail(t)

	_, stderr, code := run(t, "mail", "messages", "get", "--", convID)
	if code != 3 {
		t.Errorf("expected exit 3, got %d (stderr: %s)", code, stderr)
	}
	assertContains(t, stderr, "is a conversation, not a message")
	assertContains(t, stderr, "proton mail conversations get")
	assertContains(t, stderr, convID)
}

func TestMailConversationsReadMsgIDRedirects(t *testing.T) {
	t.Parallel()
	msgID, _, _ := plainMail(t)

	_, stderr, code := run(t, "mail", "conversations", "get", "--", msgID)
	if code != 3 {
		t.Errorf("expected exit 3, got %d (stderr: %s)", code, stderr)
	}
	assertContains(t, stderr, "is a message, not a conversation")
	assertContains(t, stderr, "proton mail messages get")
	assertContains(t, stderr, msgID)
}

func TestMailConversationsBulkDryRun(t *testing.T) {
	t.Parallel()
	_, stderr := runOKStderr(t, "--dry-run", "mail", "conversations", "trash",
		"--unread", "--limit", "3")
	assertContains(t, stderr, "Dry run")
}

func TestMailConversationsTrashRoundTrip(t *testing.T) {
	t.Parallel()
	convID := findConversationFor(t, mutableMail(t))

	runOK(t, "mail", "conversations", "trash", "--", convID)
	data := runJSON(t, "mail", "conversations", "list", "--page-size", "50")
	convs := data["conversations"].([]interface{})
	for _, c := range convs {
		if c.(map[string]interface{})["id"].(string) == convID {
			t.Error("trashed conversation should not appear in inbox list")
		}
	}
	runOK(t, "mail", "conversations", "move", "--into", "inbox", "--", convID)
}

// A thread is not the sum of its messages: Proton has its own verbs for one, and
// marking a thread read is a different request from marking each message in it
// read. So the thread-level verbs are exercised on a thread.
func TestMailConversationsMarkAndStar(t *testing.T) {
	t.Parallel()
	convID := findConversationFor(t, mutableMail(t))

	runOK(t, "mail", "conversations", "mark", "unread", "--", convID)
	if !conversationListed(t, convID, "--unread") {
		t.Error("the thread should be among the unread ones after being marked unread")
	}
	runOK(t, "mail", "conversations", "mark", "read", "--", convID)
	if conversationListed(t, convID, "--unread") {
		t.Error("the thread should not be among the unread ones after being marked read")
	}

	runOK(t, "mail", "conversations", "star", "--", convID)
	if !conversationListed(t, convID, "--folder", "starred") {
		t.Error("the thread should be starred")
	}
	runOK(t, "mail", "conversations", "unstar", "--", convID)
	if conversationListed(t, convID, "--folder", "starred") {
		t.Error("the star should be gone")
	}
}

// conversationListed reports whether a thread is in the listing the filters name.
func conversationListed(t *testing.T, convID string, filters ...string) bool {
	t.Helper()
	args := append([]string{"mail", "conversations", "list", "--page-size", "50"}, filters...)
	for _, row := range runJSONArray(t, args...) {
		if row.(map[string]interface{})["id"] == convID {
			return true
		}
	}
	return false
}

// Deleting a thread is permanent and takes the whole thread, so it is tried on
// one nothing else reads: a draft is a thread of its own, and making one sends
// nothing.
func TestMailConversationsDeleteTakesTheWholeThread(t *testing.T) {
	t.Parallel()
	subject := testID() + "-conv-delete"
	id := strings.TrimSpace(runOK(t, "mail", "drafts", "create",
		"--to", selfEmail(), "--subject", subject, "--body", "a thread of one"))
	cleanupRun(t, "Delete draft: proton mail messages delete "+id,
		"mail", "messages", "delete", "--", id)
	convID := findConversationFor(t, id)

	runOK(t, "mail", "conversations", "delete", "--", convID)

	if _, _, code := run(t, "mail", "messages", "get", "--", id); code != 3 {
		t.Errorf("the message is still there after its thread was deleted (exit %d)", code)
	}
}

// ── attachments ──

func TestMailAttachmentsListAndDownload(t *testing.T) {
	t.Parallel()
	msgID, attID, attName := findMessageWithAttachment(t)

	// List
	stdout := runOK(t, "mail", "messages", "attachments", "list", msgID)
	assertContains(t, stdout, "NAME")

	// Download to tempdir
	out := filepath.Join(t.TempDir(), "att")
	runOK(t, "mail", "messages", "attachments", "download", "--output", out, msgID, attID)

	info, err := os.Stat(out)
	if err != nil {
		t.Fatalf("attachment not saved: %v", err)
	}
	if info.Size() == 0 {
		t.Errorf("attachment %q is empty", attName)
	}
}

// findMessageWithAttachment returns a shared, delivered self-mail carrying one
// attachment (created once per suite), plus that attachment's ID and name.
func findMessageWithAttachment(t *testing.T) (msgID, attID, attName string) {
	t.Helper()
	return sharedAttachment(t)
}

func TestMailAttachmentsDownloadCollisionAutoSuffix(t *testing.T) {
	t.Parallel()
	msgID, attID, attName := findMessageWithAttachment(t)

	dir := t.TempDir()
	// Pre-create the canonical destination so the download collides.
	placeholder := filepath.Join(dir, attName)
	if err := os.WriteFile(placeholder, []byte("placeholder"), 0644); err != nil {
		t.Fatalf("setup: %v", err)
	}

	_, stderr := runOKStderr(t, "mail", "messages", "attachments", "download", "--output-dir", dir, msgID, attID)

	// Suffixed file must exist; placeholder must be untouched.
	stem := strings.TrimSuffix(attName, filepath.Ext(attName))
	ext := filepath.Ext(attName)
	suffixed := filepath.Join(dir, stem+" (2)"+ext)
	if _, err := os.Stat(suffixed); err != nil {
		t.Errorf("expected auto-suffixed file at %s, got: %v\nstderr: %s", suffixed, err, stderr)
	}
	if data, _ := os.ReadFile(placeholder); string(data) != "placeholder" {
		t.Errorf("placeholder was overwritten: %q", string(data))
	}
}

func TestMailAttachmentsDownloadCollisionExplicitErrors(t *testing.T) {
	t.Parallel()
	msgID, attID, _ := findMessageWithAttachment(t)

	dest := filepath.Join(t.TempDir(), "existing.bin")
	if err := os.WriteFile(dest, []byte("x"), 0644); err != nil {
		t.Fatalf("setup: %v", err)
	}
	_, stderr, code := run(t, "mail", "messages", "attachments", "download", "--output", dest, msgID, attID)
	if code == 0 {
		t.Errorf("expected non-zero exit on collision, got 0")
	}
	if !strings.Contains(stderr, "exists") {
		t.Errorf("expected stderr to mention 'exists', got: %s", stderr)
	}
}

func TestMailAttachmentsDownloadForce(t *testing.T) {
	t.Parallel()
	msgID, attID, _ := findMessageWithAttachment(t)

	dest := filepath.Join(t.TempDir(), "existing.bin")
	if err := os.WriteFile(dest, []byte("placeholder"), 0644); err != nil {
		t.Fatalf("setup: %v", err)
	}
	runOK(t, "mail", "messages", "attachments", "download", "--output", dest, "--force", msgID, attID)
	data, err := os.ReadFile(dest)
	if err != nil {
		t.Fatalf("read after force: %v", err)
	}
	if string(data) == "placeholder" {
		t.Error("--force did not overwrite")
	}
}

func TestMailAttachmentsDownloadAll(t *testing.T) {
	t.Parallel()
	msgID, _, _ := findMessageWithAttachment(t)

	dir := t.TempDir()
	runOK(t, "mail", "messages", "attachments", "download", "--output-dir", dir, msgID)

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("readdir: %v", err)
	}
	if len(entries) == 0 {
		t.Error("--all wrote no files")
	}
}

func TestMailAttachmentsDownloadOutputDirAutoCreates(t *testing.T) {
	t.Parallel()
	msgID, attID, _ := findMessageWithAttachment(t)

	dir := filepath.Join(t.TempDir(), "new", "deep", "nested")
	runOK(t, "mail", "messages", "attachments", "download", "--output-dir", dir, msgID, attID)

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
// findMessageWithMixedAttachments returns a shared, delivered self-mail with
// one inline image and one regular attachment (created once per suite via
// `--attach-inline`), so the inline-filter tests run for real.
func findMessageWithMixedAttachments(t *testing.T) (msgID string) {
	t.Helper()
	return sharedMixedAttachment(t)
}

func TestMailAttachmentsListFiltersInline(t *testing.T) {
	t.Parallel()
	msgID := findMessageWithMixedAttachments(t)

	// Default: filtered. Text mode must NOT have a DISPOSITION header.
	stdout := runOK(t, "mail", "messages", "attachments", "list", msgID)
	if strings.Contains(stdout, "DISPOSITION") {
		t.Error("default text-mode list should not show DISPOSITION column")
	}

	// JSON: filtered, but each entry still has the disposition field.
	defaultAtts := runJSONArray(t, "mail", "messages", "attachments", "list", msgID)
	for _, row := range defaultAtts {
		a, _ := row.(map[string]interface{})
		if d, _ := a["disposition"].(string); d == "inline" {
			t.Errorf("default list contains inline attachment: %v", a)
		}
		if _, ok := a["disposition"]; !ok {
			t.Errorf("default JSON missing disposition field on %v", a)
		}
	}

	// --include-inline: text mode shows DISPOSITION column.
	stdoutAll := runOK(t, "mail", "messages", "attachments", "list", "--include-inline", msgID)
	if !strings.Contains(stdoutAll, "DISPOSITION") {
		t.Error("--include-inline text-mode list should show DISPOSITION column")
	}

	// --include-inline JSON has both kinds.
	allAtts := runJSONArray(t, "mail", "messages", "attachments", "list", "--include-inline", msgID)
	dispositions := map[string]bool{}
	for _, row := range allAtts {
		a, _ := row.(map[string]interface{})
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
	t.Parallel()
	msgID, _, _ := findMessageWithAttachment(t)

	atts := runJSONArray(t, "mail", "messages", "attachments", "list", msgID)
	if len(atts) == 0 {
		t.Skip("no attachments after default filter")
	}
	for _, row := range atts {
		a, _ := row.(map[string]interface{})
		d, ok := a["disposition"].(string)
		if !ok {
			t.Errorf("attachment missing 'disposition' field: %v", a)
		}
		// May be "" on legacy messages; that's still a string.
		_ = d
	}
}

func TestMailAttachmentsDownloadAllSkipsInline(t *testing.T) {
	t.Parallel()
	msgID := findMessageWithMixedAttachments(t)

	dir := t.TempDir()
	runOK(t, "mail", "messages", "attachments", "download", "--output-dir", dir, msgID)

	written, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("readdir: %v", err)
	}
	numWritten := len(written)

	// --include-inline should yield strictly more files.
	dir2 := t.TempDir()
	runOK(t, "mail", "messages", "attachments", "download", "--include-inline", "--output-dir", dir2, msgID)
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
	t.Parallel()
	msgID, _ := quotedMail(t)

	default1 := runOK(t, "mail", "messages", "get", msgID)
	if !strings.Contains(default1, "ancient quoted text") {
		t.Errorf("default mode should preserve the quote; stdout:\n%s", truncateOutput(default1))
	}

	stripped := runOK(t, "mail", "messages", "get", "--strip-quotes", msgID)
	if strings.Contains(stripped, "ancient quoted text") {
		t.Errorf("--strip-quotes should remove the quote; stdout:\n%s", truncateOutput(stripped))
	}
	if !strings.Contains(stripped, "My new note") {
		t.Errorf("--strip-quotes should preserve new content; stdout:\n%s", truncateOutput(stripped))
	}
}

func TestMailMessagesReadStripQuotesNoFalsePositive(t *testing.T) {
	t.Parallel()
	msgID, _, _ := plainMail(t)

	default1 := runOK(t, "mail", "messages", "get", msgID)
	stripped := runOK(t, "mail", "messages", "get", "--strip-quotes", msgID)
	// On a body with no canonical reply marker, --strip-quotes is a no-op.
	if default1 != stripped {
		t.Errorf("--strip-quotes should be a no-op on bodies without quote markers")
	}
}

func TestMailConversationsReadSummary(t *testing.T) {
	t.Parallel()
	_, convID, _ := plainMail(t)

	data := runJSON(t, "--full-ids", "mail", "conversations", "get", convID)
	// Use the JSON shape to determine the expected message count.
	msgs := data["messages"].([]interface{})
	wantCount := len(msgs)
	if wantCount == 0 {
		t.Skip("conversation has 0 messages")
	}

	// One line per message is a table, so the rows sit under a header and a rule.
	stdout := runOK(t, "mail", "conversations", "get", "--summary", convID)
	lines := strings.Split(strings.TrimRight(stdout, "\n"), "\n")
	if len(lines) != wantCount+2 {
		t.Fatalf("expected a header, a rule and %d rows, got %d lines:\n%s", wantCount, len(lines), stdout)
	}
	for _, want := range []string{"#", "DATE", "FROM", "PREVIEW"} {
		assertContains(t, lines[0], want)
	}

	// Each row must match: <N>/<M>  <YYYY-MM-DD HH:MM>  <addr>  <preview>
	re := regexp.MustCompile(`^\d+/\d+\s+\d{4}-\d{2}-\d{2} \d{2}:\d{2}\s+\S`)
	for i, line := range lines[2:] {
		if !re.MatchString(line) {
			t.Errorf("row %d does not match summary shape: %q", i, line)
		}
	}
}

func TestMailConversationsReadSummaryAttachmentTag(t *testing.T) {
	t.Parallel()
	lease(t, attachmentThread)
	msgID, _, _ := findMessageWithAttachment(t)
	convID := findConversationFor(t, msgID)
	stdout := runOK(t, "mail", "conversations", "get", "--summary", convID)
	// A row carrying attachments says how many, in the FLAGS column.
	assertContains(t, stdout, "FLAGS")
	if !regexp.MustCompile(`(?m)\s[1-9]\d*$`).MatchString(stdout) {
		t.Errorf("expected a row flagged with an attachment count:\n%s", truncateOutput(stdout))
	}
}

func TestMailConversationsReadStripQuotesKeepsLayout(t *testing.T) {
	t.Parallel()
	_, convID, _ := plainMail(t)

	stdout := runOK(t, "mail", "conversations", "get", "--strip-quotes", convID)
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
	t.Parallel()
	msgID, _, subject := plainMail(t)
	stdout := runOK(t, "mail", "messages", "get", "--body-only", msgID)
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
	t.Parallel()
	msgID, _, _ := plainMail(t)
	stdout := runOK(t, "mail", "messages", "get", "--format", "html", msgID)
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
	t.Parallel()
	msgID, _, _ := plainMail(t)
	stdout := runOK(t, "mail", "messages", "get", "--format", "raw", msgID)
	if strings.HasPrefix(strings.TrimSpace(stdout), "Subject:") {
		t.Errorf("--format raw must not start with 'Subject:' header; got:\n%s", truncateOutput(stdout))
	}
}

func TestMailMessagesReadDefaultStillHasHeader(t *testing.T) {
	t.Parallel()
	msgID, _, _ := plainMail(t)
	stdout := runOK(t, "mail", "messages", "get", msgID)
	assertContains(t, stdout, "Subject:")
	assertContains(t, stdout, "From:")
}

func TestMailConversationsReadBodyOnly(t *testing.T) {
	t.Parallel()
	_, convID, subject := plainMail(t)

	stdout := runOK(t, "mail", "conversations", "get", "--body-only", convID)
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
	t.Parallel()
	msgID, _, _ := findMessageWithAttachment(t)
	stdout := runOK(t, "mail", "messages", "get", msgID)
	assertContains(t, stdout, "Attachments")
	assertContains(t, stdout, "NAME")
	assertContains(t, stdout, "SIZE")
}

func TestMailMessagesReadNoAttachmentsNoFooter(t *testing.T) {
	t.Parallel()
	msgID, _, _ := plainMail(t)
	stdout := runOK(t, "mail", "messages", "get", msgID)
	if strings.Contains(stdout, "---") {
		t.Errorf("unexpected '---' separator on no-attachments message:\n%s", truncateOutput(stdout))
	}
	if strings.Contains(stdout, "Attachments (") {
		t.Errorf("unexpected attachments footer on no-attachments message:\n%s", truncateOutput(stdout))
	}
}

func TestMailMessagesReadFormatHTMLNoFooter(t *testing.T) {
	t.Parallel()
	msgID, _, _ := findMessageWithAttachment(t)
	stdout := runOK(t, "mail", "messages", "get", "--format", "html", msgID)
	if strings.Contains(stdout, "Attachments (") {
		t.Errorf("--format html must not append the footer:\n%s", truncateOutput(stdout))
	}
}

func TestMailMessagesReadFormatRawNoFooter(t *testing.T) {
	t.Parallel()
	msgID, _, _ := findMessageWithAttachment(t)
	stdout := runOK(t, "mail", "messages", "get", "--format", "raw", msgID)
	if strings.Contains(stdout, "Attachments (") {
		t.Errorf("--format raw must not append the footer:\n%s", truncateOutput(stdout))
	}
}

func TestMailMessagesReadIncludeInlineTags(t *testing.T) {
	t.Parallel()
	msgID := findMessageWithMixedAttachments(t)

	// The attachments trailer is the same table the list verb draws, so an inline
	// attachment shows up as a DISPOSITION column rather than a marker.
	default1 := runOK(t, "mail", "messages", "get", msgID)
	if strings.Contains(default1, "DISPOSITION") {
		t.Errorf("default footer should not show the DISPOSITION column:\n%s", truncateOutput(default1))
	}

	incl := runOK(t, "mail", "messages", "get", "--include-inline", msgID)
	if !strings.Contains(incl, "DISPOSITION") || !strings.Contains(incl, "inline") {
		t.Errorf("--include-inline footer should show an inline attachment:\n%s", truncateOutput(incl))
	}
}

func TestMailConversationsReadShowsAttachmentsPerMessage(t *testing.T) {
	t.Parallel()
	lease(t, attachmentThread)
	msgID, _, _ := findMessageWithAttachment(t)
	convID := findConversationFor(t, msgID)
	stdout := runOK(t, "mail", "conversations", "get", convID)
	assertContains(t, stdout, "Attachments")
	assertContains(t, stdout, "NAME")
}

func TestMailConversationsReadIncludeInline(t *testing.T) {
	t.Parallel()
	lease(t, attachmentThread)
	msgID := findMessageWithMixedAttachments(t)
	convID := findConversationFor(t, msgID)

	default1 := runOK(t, "mail", "conversations", "get", convID)
	if strings.Contains(default1, "DISPOSITION") {
		t.Errorf("default conv read should not show the DISPOSITION column:\n%s", truncateOutput(default1))
	}

	incl := runOK(t, "mail", "conversations", "get", "--include-inline", convID)
	if !strings.Contains(incl, "DISPOSITION") || !strings.Contains(incl, "inline") {
		t.Errorf("--include-inline conv read should show an inline attachment:\n%s", truncateOutput(incl))
	}
}

// ── mail conversations attachments ──

func TestMailConversationsAttachmentsList(t *testing.T) {
	t.Parallel()
	lease(t, attachmentThread)
	msgID, _, _ := findMessageWithAttachment(t)
	convID := findConversationFor(t, msgID)

	// Default: filtered, includes a MESSAGE_ID column.
	stdout := runOK(t, "mail", "conversations", "attachments", "list", convID)
	assertContains(t, stdout, "MESSAGE_ID")
	if strings.Contains(stdout, "DISPOSITION") {
		t.Error("default text-mode list must not show DISPOSITION column")
	}

	// JSON: each entry carries message_id + disposition.
	atts := runJSONArray(t, "mail", "conversations", "attachments", "list", convID)
	if len(atts) == 0 {
		t.Skip("no attachments after default filter")
	}
	for _, row := range atts {
		a, _ := row.(map[string]interface{})
		if _, ok := a["message_id"].(string); !ok {
			t.Errorf("attachment missing message_id: %v", a)
		}
		if _, ok := a["disposition"]; !ok {
			t.Errorf("attachment missing disposition: %v", a)
		}
	}
}

func TestMailConversationsAttachmentsListIncludeInline(t *testing.T) {
	t.Parallel()
	lease(t, attachmentThread)
	msgID := findMessageWithMixedAttachments(t)
	convID := findConversationFor(t, msgID)

	stdout := runOK(t, "mail", "conversations", "attachments", "list",
		"--include-inline", convID)
	assertContains(t, stdout, "DISPOSITION")
	assertContains(t, stdout, "inline")
}

func TestMailConversationsAttachmentsDownloadAll(t *testing.T) {
	t.Parallel()
	lease(t, attachmentThread)
	msgID, _, _ := findMessageWithAttachment(t)
	convID := findConversationFor(t, msgID)

	dir := t.TempDir()
	runOK(t, "mail", "conversations", "attachments", "download", "--output-dir", dir, convID)

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("readdir: %v", err)
	}
	if len(entries) == 0 {
		t.Error("downloading every attachment wrote no files")
	}
}

func TestMailConversationsAttachmentsDownloadAllSkipsInline(t *testing.T) {
	t.Parallel()
	lease(t, attachmentThread)
	msgID := findMessageWithMixedAttachments(t)
	convID := findConversationFor(t, msgID)

	dir := t.TempDir()
	runOK(t, "mail", "conversations", "attachments", "download", "--output-dir", dir, convID)
	default1, _ := os.ReadDir(dir)

	dir2 := t.TempDir()
	runOK(t, "mail", "conversations", "attachments", "download", "--include-inline", "--output-dir", dir2, convID)
	inclAll, _ := os.ReadDir(dir2)

	if len(inclAll) <= len(default1) {
		t.Errorf("--include-inline (%d files) should write more than default (%d files)",
			len(inclAll), len(default1))
	}
}

func TestMailConversationsAttachmentsDownloadOneByID(t *testing.T) {
	t.Parallel()
	lease(t, attachmentThread)
	msgID, attID, attName := findMessageWithAttachment(t)
	convID := findConversationFor(t, msgID)

	dir := t.TempDir()
	runOK(t, "mail", "conversations", "attachments", "download", "--output-dir", dir, convID, attID)

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
	t.Parallel()
	lease(t, attachmentThread)
	msgID, _, _ := findMessageWithAttachment(t)
	convID := findConversationFor(t, msgID)

	_, stderr, code := run(t, "mail", "conversations", "attachments", "download",
		convID, "fake-attachment-id-"+testID())
	if code != 3 {
		t.Errorf("expected exit 3 for unknown attachment, got %d", code)
	}
	assertContains(t, stderr, "in that thread")
}

// ── labels ──

func TestMailLabelsList(t *testing.T) {
	t.Parallel()
	lease(t, labelSlot)
	name := testID() + "-list"
	id := strings.TrimSpace(runOK(t, "mail", "settings", "labels", "create", "--name", name, "--color", "#8080FF"))
	cleanupRun(t, fmt.Sprintf("Delete label: proton mail settings labels delete %s", id),
		"mail", "settings", "labels", "delete", "--", id)

	stdout := runOK(t, "mail", "settings", "labels", "list")
	assertContains(t, stdout, "NAME")
	assertContains(t, stdout, name)
}

func TestMailLabelsCreateDeleteLabel(t *testing.T) {
	t.Parallel()
	lease(t, labelSlot)
	name := testID() + "-label"

	// stdout = just the ID
	stdout := runOK(t, "mail", "settings", "labels", "create", "--name", name, "--color", "#8080FF")
	id := strings.TrimSpace(stdout)
	if !looksLikeID(id) {
		t.Fatalf("expected bare ID on stdout, got %q", stdout)
	}
	cleanupRun(t, fmt.Sprintf("Delete label: proton mail settings labels delete -- %s", id),
		"mail", "settings", "labels", "delete", "--", id)

	list := runOK(t, "mail", "settings", "labels", "list")
	assertContains(t, list, name)
}

// A folder is its own collection, not a label wearing a flag.
func TestMailFoldersCreateDelete(t *testing.T) {
	t.Parallel()
	lease(t, folderSlot)
	name := testID() + "-folder"
	stdout := runOK(t, "mail", "settings", "folders", "create", "--name", name, "--color", "#8080FF")
	id := strings.TrimSpace(stdout)
	if !looksLikeID(id) {
		t.Fatalf("expected bare ID on stdout, got %q", stdout)
	}
	cleanupRun(t, fmt.Sprintf("Delete folder: proton mail settings folders delete %s", id),
		"mail", "settings", "folders", "delete", "--", id)

	list := runOK(t, "mail", "settings", "folders", "list")
	assertContains(t, list, name)
	assertContains(t, list, "PATH")
	assertNotContains(t, runOK(t, "mail", "settings", "labels", "list"), name)
}

// ── filters ──

func TestMailFiltersCRUD(t *testing.T) {
	t.Parallel()
	lease(t, filterSlot)
	name := testID() + "-filter"
	sieve := `require ["fileinto"]; if header :contains "Subject" "xyz-never-matches-` + testID() + `" { fileinto "Archive"; }`

	stdout := runOK(t, "mail", "settings", "filters", "create", "--name", name, "--sieve", sieve)
	id := strings.TrimSpace(stdout)
	if !looksLikeID(id) {
		t.Fatalf("expected bare ID on stdout, got %q", stdout)
	}
	cleanupRun(t, fmt.Sprintf("Delete filter: proton mail settings filters delete -- %s", id),
		"mail", "settings", "filters", "delete", "--", id)

	assertContains(t, runOK(t, "mail", "settings", "filters", "list"), name)
	if got := filterStatus(t, id); got != 1 {
		t.Errorf("a new filter should be on, got status %d", got)
	}

	runOK(t, "mail", "settings", "filters", "disable", "--", id)
	if got := filterStatus(t, id); got != 0 {
		t.Errorf("status after disable = %d, want 0", got)
	}

	runOK(t, "mail", "settings", "filters", "enable", "--", id)
	if got := filterStatus(t, id); got != 1 {
		t.Errorf("status after enable = %d, want 1", got)
	}
}

// filterStatus reads one filter's on/off state, which the table spells as a yes
// or a no under ENABLED.
func filterStatus(t *testing.T, id string) int {
	t.Helper()
	for _, row := range runJSONArray(t, "--full-ids", "mail", "settings", "filters", "list") {
		f, _ := row.(map[string]interface{})
		if f["id"] == id {
			status, _ := f["status"].(float64)
			return int(status)
		}
	}
	t.Fatalf("filter %s is not in the list", id)
	return -1
}

// ── addresses ──

func TestMailAddressesList(t *testing.T) {
	t.Parallel()
	stdout := runOK(t, "mail", "settings", "addresses", "list")
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

// looksLikePairRef reports whether s is a two-part reference - the shape a Pass
// item or a calendar event is addressed by, and the shape creating one answers
// with. looksLikeID accepts it too, so asserting the halves is what tells the
// two apart.
func looksLikePairRef(s string) bool {
	left, right, ok := strings.Cut(s, "/")
	return ok && looksLikeID(left) && looksLikeID(right)
}

// findMessage polls a folder for a message with the given subject.
func findMessage(t *testing.T, folder, subject string) string {
	t.Helper()
	var id string
	waitFor(25*time.Second, 750*time.Millisecond, func() bool {
		id = messageIDInFolder(folder, subject)
		return id != ""
	})
	return id
}

// An inline image is a distinct thing to put on the wire: it is embedded in the
// HTML body by Content-ID, which is what makes Proton record the part as inline
// rather than as an attachment. The tests that tell the two dispositions apart
// read a seeded message, so the sending of one is asserted here.
func TestMailSendWithInlineImage(t *testing.T) {
	t.Parallel()
	subject := testID() + "-inline"
	dir := t.TempDir()
	img := filepath.Join(dir, "pixel.png")
	writePNG(t, img)
	note := filepath.Join(dir, "note.txt")
	if err := os.WriteFile(note, []byte("regular attachment"), 0o600); err != nil {
		t.Fatal(err)
	}

	id := strings.TrimSpace(runOK(t, "mail", "messages", "send",
		"--to", selfEmail(), "--subject", subject, "--html", "--body", "<p>see below</p>",
		"--attach", note, "--attach-inline", img))
	cleanupRun(t, "Delete sent mail: proton mail messages delete "+id,
		"mail", "messages", "delete", "--", id)
	if inbox := findMessage(t, "inbox", subject); inbox != "" {
		cleanupRun(t, "Delete inbox mail: proton mail messages delete "+inbox,
			"mail", "messages", "delete", "--", inbox)
	}

	// The image is recorded as inline and the note as an attachment, so the
	// default listing shows one of the two and --include-inline both.
	regular := runJSONArray(t, "mail", "messages", "attachments", "list", id)
	if len(regular) != 1 {
		t.Fatalf("the default listing shows %d attachments, want only the regular one", len(regular))
	}
	if name, _ := regular[0].(map[string]interface{})["name"].(string); name != "note.txt" {
		t.Errorf("the regular attachment is %q, want note.txt", name)
	}
	dispositions := map[string]bool{}
	for _, row := range runJSONArray(t, "mail", "messages", "attachments", "list", "--include-inline", id) {
		if d, ok := row.(map[string]interface{})["disposition"].(string); ok {
			dispositions[d] = true
		}
	}
	if !dispositions["inline"] || !dispositions["attachment"] {
		t.Errorf("the message carries dispositions %v, want both inline and attachment", dispositions)
	}
}

func TestMailSendWithAttachment(t *testing.T) {
	t.Parallel()
	subject := testID() + "-attach"
	dir := t.TempDir()
	path := filepath.Join(dir, "note.txt")
	content := "attachment body for " + subject
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	runOK(t, "mail", "messages", "send",
		"--to", selfEmail(), "--subject", subject,
		"--body", "see attached", "--attach", path)

	if sentID := findMessage(t, "sent", subject); sentID != "" {
		cleanupRun(t, fmt.Sprintf("Delete sent mail: proton mail messages delete %s", sentID),
			"mail", "messages", "delete", "--", sentID)
	}
	inboxID := findMessage(t, "inbox", subject)
	if inboxID == "" {
		t.Fatal("attachment mail did not arrive in inbox")
	}
	cleanupRun(t, fmt.Sprintf("Delete inbox mail: proton mail messages delete %s", inboxID),
		"mail", "messages", "delete", "--", inboxID)

	// The attachment must be listed and decrypt back to the original bytes.
	atts := runOK(t, "mail", "messages", "attachments", "list", inboxID)
	assertContains(t, atts, "note.txt")

	dlDir := filepath.Join(dir, "dl")
	if err := os.MkdirAll(dlDir, 0o755); err != nil {
		t.Fatal(err)
	}
	runOK(t, "mail", "messages", "attachments", "download", "--output-dir", dlDir, inboxID)
	got, err := os.ReadFile(filepath.Join(dlDir, "note.txt"))
	if err != nil {
		t.Fatalf("read downloaded attachment: %v", err)
	}
	if string(got) != content {
		t.Errorf("attachment round-trip mismatch: got %q want %q", got, content)
	}
}

func TestMailLabelsUpdate(t *testing.T) {
	t.Parallel()
	lease(t, labelSlot)
	name := testID() + "-label"
	id := strings.TrimSpace(runOK(t, "mail", "settings", "labels", "create", "--name", name, "--color", "#8080FF"))
	cleanupRun(t, fmt.Sprintf("Delete label: proton mail settings labels delete %s", id),
		"mail", "settings", "labels", "delete", "--", id)

	newName := name + "-renamed"
	runOK(t, "mail", "settings", "labels", "update", "--name", newName, "--color", "#DB60D6", id)
	assertContains(t, runOK(t, "mail", "settings", "labels", "list"), newName)

	// Proton replaces the whole label rather than patching it, so a change to one
	// field has to carry the rest back: a recolour must not rename, and a rename
	// must not reset the colour.
	runOK(t, "mail", "settings", "labels", "update", "--color", "#3CBB3A", id)
	assertLabel(t, id, newName, "#3CBB3A")

	again := newName + "-again"
	runOK(t, "mail", "settings", "labels", "update", "--name", again, id)
	assertLabel(t, id, again, "#3CBB3A")
}

// assertLabel checks one label's whole record, for the fields an update replaces.
func assertLabel(t *testing.T, id, name, color string) {
	t.Helper()
	for _, row := range runJSONArray(t, "mail", "settings", "labels", "list") {
		m := row.(map[string]interface{})
		if m["id"] != id {
			continue
		}
		if m["name"] != name || m["color"] != color {
			t.Errorf("label is %v/%v, want %v/%v", m["name"], m["color"], name, color)
		}
		return
	}
	t.Errorf("label %s is not in the list", id)
}

func TestMailFiltersUpdate(t *testing.T) {
	t.Parallel()
	lease(t, filterSlot)
	name := testID() + "-filter"
	sieve := `require ["fileinto"]; if header :contains "Subject" "` + name + `" { fileinto "Archive"; }`
	id := strings.TrimSpace(runOK(t, "mail", "settings", "filters", "create", "--name", name, "--sieve", sieve))
	cleanupRun(t, fmt.Sprintf("Delete filter: proton mail settings filters delete %s", id),
		"mail", "settings", "filters", "delete", "--", id)

	newName := name + "-renamed"
	runOK(t, "mail", "settings", "filters", "update", "--name", newName, id)
	assertContains(t, runOK(t, "mail", "settings", "filters", "list"), newName)
}

func TestMailSendHTMLSetsHTMLMimeType(t *testing.T) {
	t.Parallel()
	subject := testID() + "-html"
	runOK(t, "mail", "messages", "send",
		"--to", selfEmail(), "--subject", subject,
		"--body", "<p>Hello <b>world</b></p>", "--html")

	if sentID := findMessage(t, "sent", subject); sentID != "" {
		cleanupRun(t, fmt.Sprintf("Delete sent mail: proton mail messages delete %s", sentID),
			"mail", "messages", "delete", "--", sentID)
	}
	inboxID := findMessage(t, "inbox", subject)
	if inboxID == "" {
		t.Fatal("HTML mail did not arrive in inbox")
	}
	cleanupRun(t, fmt.Sprintf("Delete inbox mail: proton mail messages delete %s", inboxID),
		"mail", "messages", "delete", "--", inboxID)

	data := runJSON(t, "api", "GET", "/mail/v4/messages/"+inboxID)
	msg, ok := data["Message"].(map[string]interface{})
	if !ok {
		t.Fatalf("no Message in response: %v", data)
	}
	if mt, _ := msg["MIMEType"].(string); mt != "text/html" {
		t.Errorf("received MIMEType = %q, want text/html", mt)
	}
}

func TestMailSendPrintsMessageID(t *testing.T) {
	t.Parallel()
	subject := testID() + "-sendid"
	stdout, stderr := runOKStderr(t, "mail", "messages", "send",
		"--to", selfEmail(), "--subject", subject, "--body", "id on stdout")
	id := strings.TrimSpace(stdout)
	if !looksLikeID(id) {
		t.Fatalf("send stdout should be a bare message ID, got %q", stdout)
	}
	cleanupRun(t, "Delete sent mail: proton mail messages delete "+id,
		"mail", "messages", "delete", "--", id)
	assertContains(t, stderr, "Sent")
	// The returned ID resolves via read.
	assertContains(t, runOK(t, "mail", "messages", "get", "--", id), subject)
	// Clean the inbox copy too (distinct ID).
	if inbox := findMessage(t, "inbox", subject); inbox != "" && inbox != id {
		cleanupRun(t, "Delete inbox mail: proton mail messages delete "+inbox,
			"mail", "messages", "delete", "--", inbox)
	}
}

// A scheduled send is one behaviour, so it is one test: the message is queued
// rather than sent, it answers with the ID it was queued under, it carries the
// delivery time it was given, and it can be taken back out of the queue.
//
// Sending it once rather than three times is not only faster - a free plan meters
// what the suite may send, and that is what decides how often it can be run.
func TestMailMessagesScheduledSendAndUnschedule(t *testing.T) {
	t.Parallel()
	subject := testID() + "-scheduled"
	sendAt := time.Now().Add(3 * time.Hour)
	stdout, stderr := runOKStderr(t, "mail", "messages", "send",
		"--to", selfEmail(), "--subject", subject,
		"--body", "scheduled via --send-at", "--send-at", sendAt.Format("2006-01-02T15:04"))

	id := strings.TrimSpace(stdout)
	if !looksLikeID(id) {
		t.Fatalf("a scheduled send should print a bare message ID, got %q", stdout)
	}
	// cancel_send keeps the same message ID, so this covers it in either state.
	cleanupRun(t, "Delete scheduled mail: proton mail messages delete "+id,
		"mail", "messages", "delete", "--", id)
	assertContains(t, stderr, "Scheduled")

	// It is queued rather than sent, under exactly the ID that was reported.
	var schedID string
	waitFor(30*time.Second, 1*time.Second, func() bool {
		schedID = messageIDInFolder("scheduled", subject)
		return schedID != ""
	})
	if schedID != id {
		t.Errorf("the scheduled folder holds %q, want the ID the send reported, %q", schedID, id)
	}

	// It carries its future delivery time; an immediate send would be about now.
	data := runJSON(t, "api", "GET", "/mail/v4/messages/"+id)
	msg, _ := data["Message"].(map[string]interface{})
	msgTime, _ := msg["Time"].(float64)
	if int64(msgTime) <= time.Now().Add(time.Hour).Unix() {
		t.Errorf("scheduled Time = %d, want a delivery time near %d", int64(msgTime), sendAt.Unix())
	}

	// A dry run does not take it out of the queue.
	_, stderr = runOKStderr(t, "--dry-run", "mail", "messages", "unschedule", "--", id)
	assertContains(t, stderr, "Dry run")
	if messageIDInFolder("scheduled", subject) == "" {
		t.Error("--dry-run unscheduled the message")
	}

	// Taking it back out returns it to Drafts.
	runOK(t, "mail", "messages", "unschedule", "--", id)
	var draftID string
	waitFor(30*time.Second, 1*time.Second, func() bool {
		draftID = messageIDInFolder("drafts", subject)
		return draftID != ""
	})
	if draftID == "" {
		t.Error("an unscheduled message should appear in Drafts")
	}
	if messageIDInFolder("scheduled", subject) != "" {
		t.Error("the message is still in Scheduled after being unscheduled")
	}
}

func TestMailSendScheduledDryRunEchoesTime(t *testing.T) {
	t.Parallel()
	subject := testID() + "-scheddry"
	sendAt := time.Now().Add(5 * time.Hour)
	_, stderr := runOKStderr(t, "--dry-run", "mail", "messages", "send",
		"--to", selfEmail(), "--subject", subject,
		"--body", "x", "--send-at", sendAt.Format("2006-01-02T15:04"))
	assertContains(t, stderr, "would schedule")
	assertContains(t, stderr, sendAt.Format("2006-01-02"))
	if messageIDInFolder("scheduled", subject) != "" {
		t.Error("dry-run should not create a scheduled message")
	}
}

func TestMailMessagesUnscheduleByAllDryRun(t *testing.T) {
	t.Parallel()
	// --all with --dry-run is safe: it previews without touching the queue.
	_, stderr := runOKStderr(t, "--dry-run", "mail", "messages", "unschedule", "--all")
	assertContains(t, stderr, "Dry run")
}

func TestMailSendExpiringHasExpirationTime(t *testing.T) {
	t.Parallel()
	subject := testID() + "-expires"
	runOK(t, "mail", "messages", "send",
		"--to", selfEmail(), "--subject", subject,
		"--body", "self-destructs via --expires", "--expires", "1d")

	if sentID := findMessage(t, "sent", subject); sentID != "" {
		cleanupRun(t, fmt.Sprintf("Delete sent mail: proton mail messages delete %s", sentID),
			"mail", "messages", "delete", "--", sentID)
	}
	inboxID := findMessage(t, "inbox", subject)
	if inboxID == "" {
		t.Fatal("expiring message not delivered")
	}
	cleanupRun(t, fmt.Sprintf("Delete inbox mail: proton mail messages delete %s", inboxID),
		"mail", "messages", "delete", "--", inboxID)

	data := runJSON(t, "api", "GET", "/mail/v4/messages/"+inboxID)
	msg, _ := data["Message"].(map[string]interface{})
	exp, _ := msg["ExpirationTime"].(float64)
	now := time.Now().Unix()
	if int64(exp) <= now {
		t.Errorf("ExpirationTime = %d, expected a future expiry near %d", int64(exp), now+86400)
	}
}

func TestMailSendEncryptedForOutsideDryRun(t *testing.T) {
	t.Parallel()
	_, stderr := runOKStderr(t, "--dry-run", "mail", "messages", "send",
		"--to", externalRecipient(t), "--subject", testID()+"-eo-dry",
		"--body", "secret", "--eo-password", "hunter2", "--eo-password-hint", "the usual")
	assertContains(t, stderr, "Dry run")
}

// TestMailSendEncryptedForOutside exercises the encrypted-for-outside (password)
// send path end to end. It delivers to a real external (non-Proton) mailbox -
// an external mailbox, per tests/AGENTS.md - so a non-zero exit means either an
// EO-packaging regression or a server-side address policy. (Sending to a fake
// @example.com address instead would bounce with a MAILER-DAEMON return.)
func TestMailSendEncryptedForOutside(t *testing.T) {
	t.Parallel()
	subject := testID() + "-eo-real"

	runOK(t, "mail", "messages", "send",
		"--to", externalRecipient(t), "--subject", subject, "--body", "encrypted outside body",
		"--eo-password", "hunter2", "--eo-password-hint", "the usual")

	sentID := findMessage(t, "sent", subject)
	if sentID == "" {
		t.Fatal("EO message did not appear in Sent")
	}
	cleanupRun(t, fmt.Sprintf("Delete sent EO mail: proton mail messages delete %s", sentID),
		"mail", "messages", "delete", "--", sentID)

	// EO always attaches an expiration (defaults to 28 days).
	data := runJSON(t, "api", "GET", "/mail/v4/messages/"+sentID)
	msg, _ := data["Message"].(map[string]interface{})
	exp, _ := msg["ExpirationTime"].(float64)
	if int64(exp) <= time.Now().Unix() {
		t.Errorf("EO ExpirationTime = %d, expected a future expiry (~28 days)", int64(exp))
	}
}

func TestMailFoldersNestedReportsParent(t *testing.T) {
	t.Parallel()
	lease(t, folderSlot)
	parentName := testID() + "-parent"
	parentID := strings.TrimSpace(runOK(t, "mail", "settings", "folders", "create", "--name", parentName, "--color", "#8080FF"))
	cleanupRun(t, fmt.Sprintf("Delete parent folder: proton mail settings folders delete %s", parentID),
		"mail", "settings", "folders", "delete", "--", parentID)

	childName := testID() + "-child"
	childID := strings.TrimSpace(runOK(t, "mail", "settings", "folders", "create", "--name", childName, "--parent", parentID, "--color", "#8080FF"))
	cleanupRun(t, fmt.Sprintf("Delete child folder: proton mail settings folders delete %s", childID),
		"mail", "settings", "folders", "delete", "--", childID)

	data := runJSON(t, "api", "GET", "/core/v4/labels", "--query", "Type=3")
	labels, _ := data["Labels"].([]interface{})
	var gotParent string
	for _, l := range labels {
		m := l.(map[string]interface{})
		if m["Name"] == childName {
			gotParent, _ = m["ParentID"].(string)
		}
	}
	if gotParent != parentID {
		t.Errorf("child folder ParentID = %q, want %q", gotParent, parentID)
	}
}

// ── cross-account delivery (internal E2EE) ──
//
// Needs the second account (the `secondary` profile): the primary sends to it,
// and it decrypts the body and verifies the sender signature.

// secondaryMailContaining finds an inbox message on the second account from `from` whose
// decrypted body contains `needle`, returning its ID (or ""). Shared with the
// calendar RSVP round-trip, which checks the organizer's reply email.
func secondaryMailContaining(t *testing.T, from, needle string) string {
	t.Helper()
	list := runJSONSecondary(t, "mail", "messages", "list", "--folder", "inbox", "--page-size", "20")
	msgs, _ := list["messages"].([]interface{})
	for _, m := range msgs {
		mm := m.(map[string]interface{})
		if addr, _ := mm["from_address"].(string); addr != from {
			continue
		}
		id, _ := mm["id"].(string)
		if body, _, code := runSecondary(t, "mail", "messages", "get", "--body-only", id); code == 0 && strings.Contains(body, needle) {
			return id
		}
	}
	return ""
}

func TestMailCrossAccountDelivery(t *testing.T) {
	t.Parallel()
	subject := testID() + "-x2acct"
	body := "cross-account e2ee body for " + subject
	runOK(t, "mail", "messages", "send", "--to", secondaryEmail(), "--subject", subject, "--body", body)

	if sentID := findMessage(t, "sent", subject); sentID != "" {
		cleanupRun(t, "Delete sent mail: proton mail messages delete "+sentID,
			"mail", "messages", "delete", sentID)
	}

	// The second account receives it, decrypts the body, and the signature verifies
	// (internal Proton-to-Proton mail is signed with the sender's address key).
	var recvID string
	waitFor(45*time.Second, 3*time.Second, func() bool {
		recvID = secondaryMailContaining(t, selfEmail(), body)
		return recvID != ""
	})
	if recvID == "" {
		t.Fatal("the second account did not receive the cross-account mail")
	}
	cleanupRunSecondary(t, "Delete received mail (secondary): proton --profile secondary mail messages delete "+recvID,
		"mail", "messages", "delete", recvID)

	read := runOKSecondary(t, "mail", "messages", "get", recvID)
	assertContains(t, read, body)
	assertField(t, read, "Signature:", "verified")
}
