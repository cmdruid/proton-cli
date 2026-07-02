package mail

import (
	"encoding/base64"
	"reflect"
	"strings"
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

func genArmoredPubKey(t *testing.T) (armored, fingerprint string) {
	t.Helper()
	key, err := pgp.GenerateKey("pin", "pin@example.invalid", "x25519", 0)
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	armored, err = key.GetArmoredPublicKey()
	if err != nil {
		t.Fatalf("GetArmoredPublicKey: %v", err)
	}
	return armored, key.GetFingerprint()
}

func TestPinEncrypts(t *testing.T) {
	no, yes := false, true
	if !pinEncrypts(&PinnedRecipient{}) {
		t.Error("nil Encrypt should default to true (pinned key present)")
	}
	if !pinEncrypts(&PinnedRecipient{Encrypt: &yes}) {
		t.Error("explicit true should encrypt")
	}
	if pinEncrypts(&PinnedRecipient{Encrypt: &no}) {
		t.Error("explicit false should opt out")
	}
}

func TestPlanPinnedRecipient(t *testing.T) {
	pubA, _ := genArmoredPubKey(t)
	pubB, _ := genArmoredPubKey(t)

	t.Run("external no server key encrypts to pinned key (PGP/MIME)", func(t *testing.T) {
		pin := &PinnedRecipient{ArmoredKeys: []string{pubA}, SignatureVerified: true}
		p, err := planPinnedRecipient("bob@ext.com", schemeClear, "", pin)
		if err != nil {
			t.Fatalf("planPinnedRecipient: %v", err)
		}
		if p.scheme != schemeExternalPGP {
			t.Errorf("scheme = %d, want schemeExternalPGP", p.scheme)
		}
		if p.armoredKey != pubA {
			t.Error("expected the pinned key to be used as the send key")
		}
	})

	t.Run("pgp-inline scheme selects the inline path", func(t *testing.T) {
		pin := &PinnedRecipient{ArmoredKeys: []string{pubA}, Scheme: "pgp-inline", SignatureVerified: true}
		p, err := planPinnedRecipient("bob@ext.com", schemeClear, "", pin)
		if err != nil {
			t.Fatalf("planPinnedRecipient: %v", err)
		}
		if p.scheme != schemeExternalInline {
			t.Errorf("scheme = %d, want schemeExternalInline", p.scheme)
		}
		if p.armoredKey != pubA {
			t.Error("inline should still send to the pinned key")
		}
	})

	t.Run("internal recipient uses pinned copy of the primary key", func(t *testing.T) {
		// The primary API key is pinned (same key material), so we send to it.
		pin := &PinnedRecipient{ArmoredKeys: []string{pubA}, SignatureVerified: true}
		p, err := planPinnedRecipient("alice@proton.me", schemeInternal, pubA, pin)
		if err != nil {
			t.Fatalf("planPinnedRecipient: %v", err)
		}
		if p.scheme != schemeInternal || p.armoredKey != pubA {
			t.Errorf("got scheme=%d key-match=%v, want internal + pinned copy", p.scheme, p.armoredKey == pubA)
		}
	})

	t.Run("internal recipient whose primary key is not pinned errors", func(t *testing.T) {
		pin := &PinnedRecipient{ArmoredKeys: []string{pubB}, SignatureVerified: true}
		if _, err := planPinnedRecipient("alice@proton.me", schemeInternal, pubA, pin); err == nil {
			t.Error("expected PRIMARY_NOT_PINNED-style error when the primary key is not pinned")
		}
	})

	t.Run("unverified contact signature refuses to send", func(t *testing.T) {
		pin := &PinnedRecipient{ArmoredKeys: []string{pubA}, SignatureVerified: false}
		if _, err := planPinnedRecipient("bob@ext.com", schemeClear, "", pin); err == nil {
			t.Error("expected an error for an unverified contact signature")
		}
	})

	t.Run("no parseable pinned key errors", func(t *testing.T) {
		pin := &PinnedRecipient{ArmoredKeys: []string{"not-a-key"}, SignatureVerified: true}
		if _, err := planPinnedRecipient("bob@ext.com", schemeClear, "", pin); err == nil {
			t.Error("expected an error when no pinned key is valid for sending")
		}
	})
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

func TestBuildInlinePackage(t *testing.T) {
	recKey, err := pgp.GenerateKey("rec", "rec@ext.com", "x25519", 0)
	if err != nil {
		t.Fatalf("GenerateKey rec: %v", err)
	}
	recKR, err := pgp.NewKeyRing(recKey)
	if err != nil {
		t.Fatalf("NewKeyRing rec: %v", err)
	}
	recPub, err := recKey.GetArmoredPublicKey()
	if err != nil {
		t.Fatalf("armor rec: %v", err)
	}
	sndKey, err := pgp.GenerateKey("snd", "snd@proton.me", "x25519", 0)
	if err != nil {
		t.Fatalf("GenerateKey snd: %v", err)
	}
	sndKR, err := pgp.NewKeyRing(sndKey)
	if err != nil {
		t.Fatalf("NewKeyRing snd: %v", err)
	}
	attSK, err := pgp.GenerateSessionKey()
	if err != nil {
		t.Fatalf("attachment session key: %v", err)
	}
	atts := []*uploadedAttachment{{ID: "att-1", SessionKey: attSK}}

	// HTML input must be flattened to plaintext for an inline recipient.
	opts := SendOptions{Body: "<p>hello <b>world</b></p>", HTML: true}
	plans := []plannedRecipient{{email: "bob@ext.com", scheme: schemeExternalInline, armoredKey: recPub}}

	pkg, ok, err := New(nil).buildInlinePackage(opts, atts, plans, sndKR)
	if err != nil {
		t.Fatalf("buildInlinePackage: %v", err)
	}
	if !ok {
		t.Fatal("expected an inline package")
	}
	if pkg["Type"] != pkgPGPInline {
		t.Errorf("package Type = %v, want %d", pkg["Type"], pkgPGPInline)
	}
	if pkg["MIMEType"] != "text/plain" {
		t.Errorf("MIMEType = %v, want text/plain", pkg["MIMEType"])
	}

	addr := pkg["Addresses"].(map[string]any)["bob@ext.com"].(map[string]any)
	if addr["Type"] != pkgPGPInline {
		t.Errorf("address Type = %v, want %d", addr["Type"], pkgPGPInline)
	}

	// The body key packet is wrapped to the recipient: decrypt it, then the
	// body, and confirm the recipient reads a flattened plaintext body.
	recKP, err := base64.StdEncoding.DecodeString(addr["BodyKeyPacket"].(string))
	if err != nil {
		t.Fatalf("decode BodyKeyPacket: %v", err)
	}
	sk, err := recKR.DecryptSessionKey(recKP)
	if err != nil {
		t.Fatalf("recipient DecryptSessionKey: %v", err)
	}
	bodyData, err := base64.StdEncoding.DecodeString(pkg["Body"].(string))
	if err != nil {
		t.Fatalf("decode Body: %v", err)
	}
	dec, err := sk.Decrypt(bodyData)
	if err != nil {
		t.Fatalf("decrypt inline body: %v", err)
	}
	if got := dec.GetString(); !strings.Contains(got, "hello world") || strings.Contains(got, "<b>") {
		t.Errorf("inline body should be flattened plaintext, got %q", got)
	}

	// The attachment session key is wrapped to the recipient as well.
	akp, ok := addr["AttachmentKeyPackets"].(map[string]string)
	if !ok {
		t.Fatalf("AttachmentKeyPackets missing or wrong type: %T", addr["AttachmentKeyPackets"])
	}
	attKP, err := base64.StdEncoding.DecodeString(akp["att-1"])
	if err != nil {
		t.Fatalf("decode attachment key packet: %v", err)
	}
	gotAttSK, err := recKR.DecryptSessionKey(attKP)
	if err != nil {
		t.Fatalf("decrypt attachment session key: %v", err)
	}
	if !reflect.DeepEqual(gotAttSK.Key, attSK.Key) {
		t.Error("attachment session key was not wrapped to the recipient")
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
