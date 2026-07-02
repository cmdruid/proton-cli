package mail

import (
	"encoding/base64"
	"reflect"
	"testing"

	pgp "github.com/ProtonMail/gopenpgp/v2/crypto"
)

func TestDedupeRecipientsIsCaseInsensitiveAndPreservesFirstSeenOrder(t *testing.T) {
	got := dedupeRecipients(
		[]string{"alice@proton.me", "Bob@Example.com"},
		[]string{"alice@PROTON.me", "carol@proton.me"}, // alice is a case-insensitive dup
		[]string{"bob@example.com"},                    // bob is a dup
	)
	want := []string{"alice@proton.me", "Bob@Example.com", "carol@proton.me"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("dedupeRecipients = %v, want %v", got, want)
	}
}

func TestClassifyRecipient(t *testing.T) {
	const pw = "hunter2"
	internalKey := apiPublicKey{PublicKey: "INTERNAL", Flags: 3, Source: 0}         // NOT_OBSOLETE|NOT_COMPROMISED, Proton
	noEncryptKey := apiPublicKey{PublicKey: "DISABLED", Flags: 3 | 4, Source: 0}    // e2ee disabled for mail
	wkdKey := apiPublicKey{PublicKey: "WKD", Flags: 3, Source: 1}                   // external WKD key
	unverifiedInternal := apiPublicKey{PublicKey: "UNVER-INT", Flags: 3, Source: 0} // unverified Proton key

	tests := []struct {
		name       string
		resp       keysAllResponse
		eoPassword string
		wantScheme sendScheme
		wantKey    string
	}{
		{
			name:       "internal address key",
			resp:       resp(addr(internalKey), nil),
			wantScheme: schemeInternal, wantKey: "INTERNAL",
		},
		{
			name:       "internal via unverified proton key",
			resp:       resp(nil, []apiPublicKey{unverifiedInternal}),
			wantScheme: schemeInternal, wantKey: "UNVER-INT",
		},
		{
			name:       "external PGP from unverified WKD key",
			resp:       resp(nil, []apiPublicKey{wkdKey}),
			wantScheme: schemeExternalPGP, wantKey: "WKD",
		},
		{
			name:       "no key + eo password => EO",
			resp:       resp(nil, nil),
			eoPassword: pw,
			wantScheme: schemeEO,
		},
		{
			name:       "no key, no password => cleartext",
			resp:       resp(nil, nil),
			wantScheme: schemeClear,
		},
		{
			name:       "e2ee-disabled internal key is not mail-capable => cleartext",
			resp:       resp(addr(noEncryptKey), nil),
			wantScheme: schemeClear,
		},
		{
			name:       "e2ee-disabled internal key + password => EO",
			resp:       resp(addr(noEncryptKey), nil),
			eoPassword: pw,
			wantScheme: schemeEO,
		},
		{
			name:       "internal wins over an external WKD key",
			resp:       resp(addr(internalKey), []apiPublicKey{wkdKey}),
			wantScheme: schemeInternal, wantKey: "INTERNAL",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			gotScheme, gotKey := classifyRecipient(tc.resp, tc.eoPassword)
			if gotScheme != tc.wantScheme {
				t.Errorf("scheme = %d, want %d", gotScheme, tc.wantScheme)
			}
			if gotKey != tc.wantKey {
				t.Errorf("key = %q, want %q", gotKey, tc.wantKey)
			}
		})
	}
}

func addr(keys ...apiPublicKey) []apiPublicKey { return keys }

func resp(addressKeys, unverifiedKeys []apiPublicKey) keysAllResponse {
	var r keysAllResponse
	r.Address.Keys = addressKeys
	r.Unverified.Keys = unverifiedKeys
	return r
}

func TestMailCapable(t *testing.T) {
	if !mailCapable(3) {
		t.Error("flags 3 (NOT_OBSOLETE|NOT_COMPROMISED) should be mail-capable")
	}
	if mailCapable(3 | keyFlagEmailNoEncrypt) {
		t.Error("FLAG_EMAIL_NO_ENCRYPT should make a key non-mail-capable")
	}
}

// TestAttachmentPasswordKeyPacketsRoundTrip proves an EO attachment key packet
// decrypts back to the original session key with the password.
func TestAttachmentPasswordKeyPacketsRoundTrip(t *testing.T) {
	const pw = "correct horse battery staple"
	sk, err := pgp.GenerateSessionKey()
	if err != nil {
		t.Fatalf("GenerateSessionKey: %v", err)
	}
	atts := []*uploadedAttachment{{ID: "att-1", SessionKey: sk}}

	out, err := attachmentPasswordKeyPackets(atts, pw)
	if err != nil {
		t.Fatalf("attachmentPasswordKeyPackets: %v", err)
	}
	packetB64, ok := out["att-1"]
	if !ok {
		t.Fatalf("missing key packet for att-1: %v", out)
	}
	packet, err := base64.StdEncoding.DecodeString(packetB64)
	if err != nil {
		t.Fatalf("packet is not base64: %v", err)
	}
	got, err := pgp.DecryptSessionKeyWithPassword(packet, []byte(pw))
	if err != nil {
		t.Fatalf("DecryptSessionKeyWithPassword: %v", err)
	}
	if !reflect.DeepEqual(got.Key, sk.Key) {
		t.Error("round-tripped session key does not match the original")
	}
	if _, err := pgp.DecryptSessionKeyWithPassword(packet, []byte("wrong")); err == nil {
		t.Error("decrypting with the wrong password should fail")
	}
}

func TestAttachmentPasswordKeyPacketsEmpty(t *testing.T) {
	out, err := attachmentPasswordKeyPackets(nil, "pw")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out != nil {
		t.Errorf("expected nil for no attachments, got %v", out)
	}
}

func TestRecipientListPairsAddressAndName(t *testing.T) {
	got := recipientList([]string{"jane@proton.me"})
	if len(got) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(got))
	}
	if got[0]["Address"] != "jane@proton.me" || got[0]["Name"] != "jane@proton.me" {
		t.Errorf("recipientList entry = %v", got[0])
	}
	if recipientList(nil) == nil {
		t.Error("recipientList(nil) should return a non-nil empty slice for JSON encoding")
	}
}
