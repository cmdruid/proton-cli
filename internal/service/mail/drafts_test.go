package mail

import (
	"testing"

	"github.com/cmdruid/proton-cli/internal/account/keys"
)

func TestDraftPayloadCarriesTheSenderIdentity(t *testing.T) {
	c := Content{
		From: &Sender{Address: keys.Address{
			ID: "addr-1", Email: "jane@proton.me", DisplayName: "Jane Roe",
		}},
		To:      ParseRecipients([]string{"bob@example.com"}),
		CC:      ParseRecipients([]string{"cc@example.com"}),
		Subject: "Hi",
	}
	msg := draftPayload(c, "-----BEGIN PGP MESSAGE-----")

	sender, ok := msg["Sender"].(map[string]string)
	if !ok {
		t.Fatalf("Sender is %T, want map[string]string", msg["Sender"])
	}
	if sender["Address"] != "jane@proton.me" || sender["Name"] != "Jane Roe" {
		t.Errorf("Sender = %v, want the address paired with its display name", sender)
	}
	// The draft must be filed against the sending address, or Proton stores it
	// under the wrong identity.
	if msg["AddressID"] != "addr-1" {
		t.Errorf("AddressID = %v, want addr-1", msg["AddressID"])
	}
	if msg["Subject"] != "Hi" || msg["MIMEType"] != mimeTypePlain {
		t.Errorf("Subject/MIMEType = %v/%v", msg["Subject"], msg["MIMEType"])
	}
	if len(msg["ToList"].([]map[string]string)) != 1 {
		t.Errorf("ToList = %v", msg["ToList"])
	}
	if len(msg["BCCList"].([]map[string]string)) != 0 {
		t.Errorf("BCCList should be present but empty, got %v", msg["BCCList"])
	}
}

// An address with no display name falls back to its own address, so recipients
// never see an empty From name.
func TestDraftPayloadFallsBackToTheAddressForTheName(t *testing.T) {
	c := Content{From: &Sender{Address: keys.Address{Email: "jane@proton.me"}}, HTML: true}
	msg := draftPayload(c, "")
	if got := msg["Sender"].(map[string]string)["Name"]; got != "jane@proton.me" {
		t.Errorf("Sender name = %q, want the address", got)
	}
	if msg["MIMEType"] != mimeTypeHTML {
		t.Errorf("MIMEType = %v, want %s", msg["MIMEType"], mimeTypeHTML)
	}
}

func TestNormalizeContentIDStripsBrackets(t *testing.T) {
	for in, want := range map[string]string{
		"<abc@proton.me>": "abc@proton.me",
		"abc@proton.me":   "abc@proton.me",
		"":                "",
	} {
		if got := normalizeContentID(in); got != want {
			t.Errorf("normalizeContentID(%q) = %q, want %q", in, got, want)
		}
	}
}

// A forward carries only what identifies an attachment and unwraps its key; the
// server re-creates it with its own ID, which is why nothing else travels.
func TestCarriedAttachmentCarriesIdentityAndKeyOnly(t *testing.T) {
	got := carriedFrom(&rawMessage{Attachments: []rawAttachment{
		{ID: "att-1", Name: "report.pdf", KeyPackets: "cGFja2V0", MIMEType: "application/pdf", Size: 99},
	}})
	if len(got) != 1 {
		t.Fatalf("got %d carried attachments", len(got))
	}
	if got[0].ID != "att-1" || got[0].Name != "report.pdf" || got[0].KeyPackets != "cGFja2V0" {
		t.Errorf("carried = %+v", got[0])
	}
}

func TestDraftAttachmentListExposesInlineState(t *testing.T) {
	d := &Draft{Attachments: []*draftAttachment{
		{ID: "a1", Name: "logo.png", Size: 10, MIMEType: "image/png", ContentID: "cid1"},
		{ID: "a2", Name: "report.pdf", Size: 20, MIMEType: "application/pdf"},
	}}
	got := d.AttachmentList()
	if len(got) != 2 {
		t.Fatalf("got %d attachments", len(got))
	}
	if !got[0].Inline || got[1].Inline {
		t.Errorf("inline flags = %v / %v", got[0].Inline, got[1].Inline)
	}
	if got[1].Name != "report.pdf" || got[1].Size != 20 {
		t.Errorf("attachment = %+v", got[1])
	}
}
