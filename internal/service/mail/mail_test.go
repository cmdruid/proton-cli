package mail

import (
	"context"
	"net/url"
	"testing"

	pgp "github.com/ProtonMail/gopenpgp/v2/crypto"
	pgphelper "github.com/roman-16/proton-cli/internal/crypto/pgp"
	"github.com/roman-16/proton-cli/internal/proton"
)

func genMailKeyRing(t *testing.T) *pgp.KeyRing {
	t.Helper()
	key, err := pgp.GenerateKey("test", "test@example.invalid", "x25519", 0)
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	kr, err := pgp.NewKeyRing(key)
	if err != nil {
		t.Fatalf("NewKeyRing: %v", err)
	}
	return kr
}

// TestDecryptBody covers the verdict mapping and the key correctness property:
// gopenpgp returns the decrypted body alongside a signature error, so a body
// must still be recovered even when its signature cannot be verified.
func TestDecryptBody(t *testing.T) {
	kr := genMailKeyRing(t)
	other := genMailKeyRing(t)
	const plain = "secret body"

	enc, err := kr.Encrypt(pgp.NewPlainMessageFromString(plain), kr) // encrypt+sign with kr
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}
	armored, err := enc.GetArmored()
	if err != nil {
		t.Fatalf("GetArmored: %v", err)
	}

	tests := []struct {
		name     string
		verifier *pgp.KeyRing
		want     pgphelper.VerifyResult
	}{
		{"correct verifier verifies", kr, pgphelper.Verified},
		{"no verifier is unverified", nil, pgphelper.Unverified},
		{"unrelated verifier is unverified", other, pgphelper.Unverified},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			body, v, err := decryptBody(armored, kr, tc.verifier)
			if err != nil {
				t.Fatalf("decryptBody: %v", err)
			}
			if body != plain {
				t.Errorf("body = %q, want %q (must survive signature outcome)", body, plain)
			}
			if v != tc.want {
				t.Errorf("verdict = %q, want %q", v, tc.want)
			}
		})
	}
}

func TestResolveFolder(t *testing.T) {
	tests := []struct{ in, want string }{
		{"inbox", "0"},
		{"INBOX", "0"},
		{"trash", "3"},
		{"all", "5"},
		{"starred", "10"},
		{"Sent", "7"},
		{"some-custom-label-id==", "some-custom-label-id=="}, // passthrough
	}
	for _, tc := range tests {
		if got := ResolveFolder(tc.in); got != tc.want {
			t.Errorf("ResolveFolder(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestOppositeKind(t *testing.T) {
	if OppositeKind("conversation") != "message" {
		t.Error("OppositeKind(conversation) should be message")
	}
	if OppositeKind("message") != "conversation" {
		t.Error("OppositeKind(message) should be conversation")
	}
}

func TestSearchQueryDefaults(t *testing.T) {
	q := searchQuery(SearchOptions{Limit: 25}, false)
	if q.Get("LabelID") != "5" { // empty folder defaults to "all"
		t.Errorf("LabelID = %q, want 5 (all)", q.Get("LabelID"))
	}
	if q.Get("Sort") != "Time" || q.Get("Desc") != "1" {
		t.Errorf("expected Sort=Time Desc=1, got Sort=%q Desc=%q", q.Get("Sort"), q.Get("Desc"))
	}
	if q.Get("PageSize") != "25" {
		t.Errorf("PageSize = %q, want 25", q.Get("PageSize"))
	}
}

func TestSearchQueryFieldMapping(t *testing.T) {
	opts := SearchOptions{
		Keyword: "invoice", From: "a@x.com", To: "b@x.com",
		Subject: "hi", Folder: "inbox", Limit: 10, Unread: true,
	}
	q := searchQuery(opts, false)
	checks := map[string]string{
		"LabelID": "0",
		"Keyword": "invoice",
		"From":    "a@x.com",
		"To":      "b@x.com",
		"Subject": "hi",
		"Unread":  "1",
	}
	for k, want := range checks {
		if q.Get(k) != want {
			t.Errorf("%s = %q, want %q", k, q.Get(k), want)
		}
	}
	if q.Has("Recipients") {
		t.Error("messages search must not set Recipients")
	}
}

func TestSearchQueryRecipientsForConversations(t *testing.T) {
	q := searchQuery(SearchOptions{To: "b@x.com", Limit: 5}, true)
	if q.Get("Recipients") != "b@x.com" {
		t.Errorf("conversations search should map To→Recipients, got %q", q.Get("Recipients"))
	}
	if q.Has("To") {
		t.Error("conversations search must not set To")
	}
}

func TestValidateDates(t *testing.T) {
	q := url.Values{}
	if err := validateDates(SearchOptions{After: "2020-01-01", Before: "2099-12-31"}, q); err != nil {
		t.Fatalf("valid dates errored: %v", err)
	}
	if q.Get("Begin") == "" || q.Get("End") == "" {
		t.Errorf("expected Begin and End set, got Begin=%q End=%q", q.Get("Begin"), q.Get("End"))
	}

	if err := validateDates(SearchOptions{After: "nope"}, url.Values{}); err == nil {
		t.Error("invalid --after should error")
	}
	if err := validateDates(SearchOptions{Before: "nope"}, url.Values{}); err == nil {
		t.Error("invalid --before should error")
	}
}

func TestToMessageMapping(t *testing.T) {
	raw := rawListMessage{ID: "id1", Subject: "Hi", Unread: 1, Time: 42, NumAttachments: 2}
	raw.Sender.Name = "Alice"
	raw.Sender.Address = "alice@x.com"
	m := toMessage(raw)
	if m.ID != "id1" || m.Subject != "Hi" || m.Unread != 1 || m.Time != 42 || m.NumAttachments != 2 {
		t.Errorf("toMessage scalar mapping wrong: %+v", m)
	}
	if m.FromName != "Alice" || m.FromAddress != "alice@x.com" {
		t.Errorf("toMessage sender mapping wrong: %+v", m)
	}
}

func TestToConversationMapping(t *testing.T) {
	raw := rawConversation{ID: "c1", Subject: "Thread", NumMessages: 3, NumUnread: 1, NumAttachments: 0, Time: 99}
	raw.Labels = []struct{ ID string }{{ID: "0"}, {ID: "5"}}
	c := toConversation(raw)
	if c.ID != "c1" || c.Subject != "Thread" || c.NumMessages != 3 || c.NumUnread != 1 || c.Time != 99 {
		t.Errorf("toConversation scalar mapping wrong: %+v", c)
	}
	if len(c.Labels) != 2 || c.Labels[0] != "0" || c.Labels[1] != "5" {
		t.Errorf("toConversation labels mapping wrong: %v", c.Labels)
	}
}

func TestLooksLikeID(t *testing.T) {
	full := "NWM5AYGxFIHWT2_QbBr-whe-bIE8rbZunzr5RhXGaihvQ43z2qcxcqFgVRwi7A5C-ADmohv7TjXfYbDEIHZPQ=="
	if !LooksLikeID(full) {
		t.Error("LooksLikeID should accept a full Proton ID")
	}
	if LooksLikeID("invoice") {
		t.Error("LooksLikeID should reject a short search term")
	}
}

// fakeDoer captures the last request issued through the proton.Doer seam.
type fakeDoer struct{ last proton.Request }

func (f *fakeDoer) Do(_ context.Context, r proton.Request) (*proton.Response, error) {
	f.last = r
	return &proton.Response{Status: 200, Body: []byte(`{"Code":1000}`)}, nil
}

func (f *fakeDoer) Decode(_ context.Context, r proton.Request, _ any) error {
	f.last = r
	return nil
}

func bodyLabelID(t *testing.T, r proton.Request) string {
	t.Helper()
	body, ok := r.Body.(map[string]any)
	if !ok {
		t.Fatalf("request body is not map[string]any: %T", r.Body)
	}
	id, _ := body["LabelID"].(string)
	return id
}

func TestTrashHitsLabelEndpoint(t *testing.T) {
	f := &fakeDoer{}
	s := New(f)
	if err := s.Trash(context.Background(), []string{"a", "b"}); err != nil {
		t.Fatalf("Trash: %v", err)
	}
	if f.last.Method != "PUT" || f.last.Path != "/mail/v4/messages/label" {
		t.Errorf("Trash issued %s %s", f.last.Method, f.last.Path)
	}
	if got := bodyLabelID(t, f.last); got != labelTrash {
		t.Errorf("Trash LabelID = %q, want %q", got, labelTrash)
	}
}

func TestMoveResolvesFolderAlias(t *testing.T) {
	f := &fakeDoer{}
	s := New(f)
	if err := s.Move(context.Background(), []string{"a"}, "archive"); err != nil {
		t.Fatalf("Move: %v", err)
	}
	if got := bodyLabelID(t, f.last); got != labelArchive {
		t.Errorf("Move to archive LabelID = %q, want %q", got, labelArchive)
	}
}
