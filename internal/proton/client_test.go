package proton

import "testing"

func TestSessionPrefersEncKeyBlob(t *testing.T) {
	c := New(Options{})
	c.SetTokens("uid", "acc", "ref")
	c.SetSaltedKeyPass("legacy-cleartext")
	c.SetEncKeyBlob("encrypted-blob")

	s := c.Session()
	if s.UID != "uid" || s.AccessToken != "acc" || s.RefreshToken != "ref" {
		t.Errorf("tokens not carried: %+v", s)
	}
	if s.EncKeyBlob != "encrypted-blob" {
		t.Errorf("EncKeyBlob = %q, want encrypted-blob", s.EncKeyBlob)
	}
	if s.SaltedKeyPass != "" {
		t.Errorf("once a blob exists, cleartext must be dropped; got %q", s.SaltedKeyPass)
	}
}

func TestSessionPreservesLegacyUntilMigrated(t *testing.T) {
	c := New(Options{})
	c.SetTokens("uid", "acc", "ref")
	c.SetSaltedKeyPass("legacy-cleartext")

	s := c.Session()
	if s.EncKeyBlob != "" {
		t.Errorf("EncKeyBlob should be empty, got %q", s.EncKeyBlob)
	}
	if s.SaltedKeyPass != "legacy-cleartext" {
		t.Errorf("legacy cleartext should be preserved until migrated, got %q", s.SaltedKeyPass)
	}
}

func TestSessionNoKeyMaterial(t *testing.T) {
	c := New(Options{})
	c.SetTokens("uid", "acc", "ref")
	s := c.Session()
	if s.EncKeyBlob != "" || s.SaltedKeyPass != "" {
		t.Errorf("expected no key material, got blob=%q skp=%q", s.EncKeyBlob, s.SaltedKeyPass)
	}
}
