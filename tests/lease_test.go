package tests

import (
	"os"
	"regexp"
	"slices"
	"strings"
	"sync"
	"testing"
)

// What two tests cannot both have at once.
//
// The suite runs tests at the same time, which is safe for almost everything:
// each test makes its own labels, folders, events and items under its own
// testID(), and asserts on those. What is not safe is the handful of things an
// account has exactly one of - its settings, an address's signature, the
// auto-reply - and the handful of tests that identify their own work by comparing
// a listing before and after, which another test's work would appear in.
//
// Those are named here and leased. A lease is not a hint: the test named below
// holds it for its whole run, so two tests that would tread on each other are
// ordered, while everything unrelated keeps going. This is what `t.Parallel()`
// alone cannot express - two tests that exclude each other but nobody else.
//
// Adding a shared thing means adding it here and leasing it in every test that
// touches it. TestEveryTestThatTouchesSharedStateLeasesIt checks that.
const (
	// accountSettings, mailSettings: an account has one of each, and the tests
	// that change one read it first and put it back.
	accountSettings = "account-settings"
	mailSettings    = "mail-settings"
	// addressIdentity is an address's signature and display name. Two tests
	// change the signature and each restores what it found.
	addressIdentity = "address-identity"
	// autoReplySetting is one per account, and an armed auto-reply would answer
	// the suite's own mail.
	autoReplySetting = "auto-reply"
	// photos: a photo has no name in a listing, so a test finds the one it
	// uploaded by comparing the listing before and after. Another upload in
	// between would be indistinguishable from its own.
	photos = "photos"
	// driveInvitations: likewise for the invitations the second account can see.
	driveInvitations = "drive-invitations"
	// pinnedKeys: a pinned key is looked up by address across every contact, not
	// per contact, so a test that pins the right key for the second account and
	// one that pins a wrong key for it decide each other's answer - one asserts a
	// send goes through and the other that it is refused. A test pinning for an
	// address of its own shares nothing and needs no lease.
	pinnedKeys = "pinned-keys"
	// passLibrary: an export holds every item in the account and reading one back
	// adds all of them, so a test that does the round trip makes a copy of
	// everything and has to remove everything it copied. Both halves are account
	// wide: another test's item would be caught by the copy, and then by the
	// clean-up.
	passLibrary = "pass-library"
	// driveTrash: emptying it acts on everything in it, including what another
	// test trashed and means to restore.
	driveTrash = "drive-trash"
	// attachmentThread: forwarding the seeded message with attachments puts a
	// draft in its thread, and a conversation Proton has just had to re-thread
	// answers for a moment as though none of its messages had any attachments.
	// A test reading that thread cannot tell that from the truth.
	attachmentThread = "attachment-thread"
	// The free plan allows only a few calendars, vaults, labels and mail folders,
	// and the fixture already holds one of each. A test that makes one takes the
	// spare slot: two at once is how a run learns that Proton counts them, in the
	// form of a test failing somewhere it has nothing to do with.
	calendarSlot = "calendar-slot"
	vaultSlot    = "vault-slot"
	labelSlot    = "label-slot"
	folderSlot   = "folder-slot"
	filterSlot   = "filter-slot"
	// calendarDefaults are per-calendar and every test shares the Default one,
	// so two tests changing what a new event inherits would each see the other's
	// value and put back the wrong one.
	calendarDefaults = "calendar-defaults"
	// sharedAlias: making one is what Proton meters hardest, so the suite seeds a
	// single alias and the tests that need one read it. The ones that change it -
	// switching it off, pointing it somewhere else, hanging contacts on it - take
	// turns, and each puts back what it changed.
	sharedAlias = "shared-alias"
	// mailTrash: emptying it removes everything in it, including what another
	// test trashed and means to find again.
	mailTrash = "mail-trash"
	// sending is held for the moment a message goes out - see runAs, which takes
	// it for every command that puts one on the wire. Sending is what a free plan
	// meters and what Proton judges an account by, so the suite never sends two at
	// once. It is not held for the wait that follows, because what has to be
	// spaced is the sending.
	sending = "sending"
)

var (
	leasesMu sync.Mutex
	leases   = map[string]*sync.Mutex{}
)

func leaseFor(name string) *sync.Mutex {
	leasesMu.Lock()
	defer leasesMu.Unlock()
	m, ok := leases[name]
	if !ok {
		m = &sync.Mutex{}
		leases[name] = m
	}
	return m
}

// lease takes exclusive use of the named shared state until the test ends.
//
// The names are taken in a fixed order, so two tests that lease the same pair
// cannot each hold half of it.
func lease(t *testing.T, names ...string) {
	t.Helper()
	slices.Sort(names)
	for _, name := range slices.Compact(names) {
		m := leaseFor(name)
		m.Lock()
		t.Cleanup(m.Unlock)
	}
}

// holding runs fn with the named state leased, for a caller that needs it for
// less than the whole test.
func holding(name string, fn func()) {
	m := leaseFor(name)
	m.Lock()
	defer m.Unlock()
	fn()
}

// ── the rules above, checked ──

// touching declares how to tell, from a test's source, that it touches something
// shared. A test matches when every phrase appears in it.
var touching = []struct {
	resource string
	phrases  []string
}{
	{mailSettings, []string{`"mail", "settings", "set"`}},
	{accountSettings, []string{`"account", "settings", "set"`}},
	{addressIdentity, []string{`"--signature"`}},
	{addressIdentity, []string{`"--clear-signature"`}},
	{autoReplySetting, []string{`"autoreply", "set"`}},
	{autoReplySetting, []string{`"autoreply", "enable"`}},
	{autoReplySetting, []string{`"autoreply", "disable"`}},
	{calendarSlot, []string{`"calendars", "create"`}},
	{calendarDefaults, []string{`"calendars", "update"`, `"--default-duration"`}},
	{calendarDefaults, []string{`"calendars", "update"`, `"--busy"`}},
	{calendarDefaults, []string{`"calendars", "update"`, `"--remind"`}},
	{vaultSlot, []string{`"vaults", "create"`}},
	{labelSlot, []string{`"settings", "labels", "create"`}},
	{labelSlot, []string{`"POST", "/core/v4/labels"`}},
	{folderSlot, []string{`"settings", "folders", "create"`}},
	{filterSlot, []string{`"filters", "create"`}},
	{photos, []string{`photoLinkIDs(t)`}},
	{driveInvitations, []string{`altInvitationIDs(t)`}},
	{passLibrary, []string{`passItemRefs(t)`}},
	{sharedAlias, []string{`seededAlias(t)`, `"aliases", "disable"`}},
	{sharedAlias, []string{`seededAlias(t)`, `"items", "update"`}},
	{sharedAlias, []string{`seededAlias(t)`, `"contacts", "create"`}},
	{pinnedKeys, []string{`"keys", "pin"`, `secondaryEmail()`}},
	{attachmentThread, []string{`sharedAttachment(t)`, `"forward"`}},
	{attachmentThread, []string{`findMessageWithAttachment(t)`, `"conversations"`}},
	{attachmentThread, []string{`findMessageWithMixedAttachments(t)`, `"conversations"`}},
	{driveTrash, []string{`"drive", "trash", "empty"`}},
	{driveTrash, []string{`"drive", "trash", "restore"`}},
	{mailTrash, []string{`"messages", "empty"`}},
	{driveInvitations, []string{`"share", "add"`, `secondaryEmail()`}},
}

// paidTestFile holds the tests that act on the paid account. See testBodies.
const paidTestFile = "paid_on_test.go"

// serialTests are the tests that run alone, and why. A test without
// `t.Parallel()` runs to completion before any parallel test resumes, which is
// how a test gets the whole account to itself.
var serialTests = map[string]string{
	"TestShortIDAmbiguousErrors": "it replaces the ID cache file that every other invocation writes to",
}

func TestEveryTestThatTouchesSharedStateLeasesIt(t *testing.T) {
	t.Parallel()
	for name, body := range testBodies(t) {
		for _, s := range touching {
			if !containsAll(body, s.phrases) {
				continue
			}
			if !strings.Contains(body, "lease(t, "+resourceConst(s.resource)+")") {
				t.Errorf("%s touches %s but does not lease it; add lease(t, %s)",
					name, s.resource, resourceConst(s.resource))
			}
		}
	}
}

func TestEveryTestRunsInParallelUnlessItSaysWhy(t *testing.T) {
	t.Parallel()
	bodies := testBodies(t)
	for name, body := range bodies {
		if _, declared := serialTests[name]; declared {
			if strings.Contains(body, "t.Parallel()") {
				t.Errorf("%s is declared serial in serialTests but calls t.Parallel()", name)
			}
			continue
		}
		if !strings.Contains(body, "t.Parallel()") {
			t.Errorf("%s does not call t.Parallel(); add it, or say why in serialTests", name)
		}
	}
	for name := range serialTests {
		if _, ok := bodies[name]; !ok {
			t.Errorf("serialTests names %s, which no longer exists", name)
		}
	}
}

// resourceConst is the identifier a lease is written with, from the value it
// holds, so the error names what to paste.
func resourceConst(resource string) string {
	switch resource {
	case accountSettings:
		return "accountSettings"
	case mailSettings:
		return "mailSettings"
	case addressIdentity:
		return "addressIdentity"
	case autoReplySetting:
		return "autoReplySetting"
	case photos:
		return "photos"
	case driveInvitations:
		return "driveInvitations"
	case passLibrary:
		return "passLibrary"
	case pinnedKeys:
		return "pinnedKeys"
	case driveTrash:
		return "driveTrash"
	case mailTrash:
		return "mailTrash"
	case attachmentThread:
		return "attachmentThread"
	case calendarSlot:
		return "calendarSlot"
	case calendarDefaults:
		return "calendarDefaults"
	case vaultSlot:
		return "vaultSlot"
	case labelSlot:
		return "labelSlot"
	case folderSlot:
		return "folderSlot"
	case filterSlot:
		return "filterSlot"
	}
	return resource
}

func containsAll(body string, phrases []string) bool {
	for _, p := range phrases {
		if !strings.Contains(body, p) {
			return false
		}
	}
	return true
}

// testBodies is every test in the live suite, by name, as source.
//
// The paid tests are left out. What is leased here is shared state on the two
// free accounts, and a paid test acts on a different account entirely, in a run
// of its own that goes one test at a time - so it can race nothing, and the
// words it happens to contain are not a claim on a free account's calendars.
func testBodies(t *testing.T) map[string]string {
	t.Helper()
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("read the test directory: %v", err)
	}
	// A body ends where the next declaration begins, whatever it is: reading on to
	// the next test would take any helper in between with it.
	decl := regexp.MustCompile(`(?m)^func `)
	test := regexp.MustCompile(`^func (Test\w+)\(t \*testing\.T\) \{`)
	bodies := map[string]string{}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), "_test.go") || e.Name() == paidTestFile {
			continue
		}
		src, err := os.ReadFile(e.Name())
		if err != nil {
			t.Fatalf("read %s: %v", e.Name(), err)
		}
		found := decl.FindAllStringSubmatchIndex(string(src), -1)
		for i, m := range found {
			end := len(src)
			if i+1 < len(found) {
				end = found[i+1][0]
			}
			body := string(src[m[0]:end])
			signature := test.FindStringSubmatch(body)
			if signature == nil {
				continue
			}
			bodies[signature[1]] = body
		}
	}
	if len(bodies) == 0 {
		t.Fatal("found no tests to check")
	}
	return bodies
}
