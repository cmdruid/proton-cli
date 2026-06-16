package aead

import (
	"bytes"
	"testing"
)

func mustKey(t *testing.T) []byte {
	t.Helper()
	k, err := NewKey()
	if err != nil {
		t.Fatalf("NewKey: %v", err)
	}
	if len(k) != KeyLen {
		t.Fatalf("NewKey len = %d, want %d", len(k), KeyLen)
	}
	return k
}

func TestRoundTrip(t *testing.T) {
	key := mustKey(t)
	plain := []byte("the quick brown fox")
	aad := []byte(TagItemContent)

	ct, err := Encrypt(key, plain, aad)
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}
	// Layout is [12-byte IV | ciphertext+16-byte tag].
	if want := IVLen + len(plain) + 16; len(ct) != want {
		t.Errorf("ciphertext len = %d, want %d", len(ct), want)
	}
	if bytes.Contains(ct, plain) {
		t.Error("ciphertext contains plaintext")
	}

	got, err := Decrypt(key, ct, aad)
	if err != nil {
		t.Fatalf("Decrypt: %v", err)
	}
	if !bytes.Equal(got, plain) {
		t.Errorf("round-trip = %q, want %q", got, plain)
	}
}

func TestRoundTripEmptyPlaintext(t *testing.T) {
	key := mustKey(t)
	ct, err := Encrypt(key, nil, nil)
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}
	got, err := Decrypt(key, ct, nil)
	if err != nil {
		t.Fatalf("Decrypt: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("empty plaintext round-trip = %q, want empty", got)
	}
}

func TestDecryptWrongKeyFails(t *testing.T) {
	ct, err := Encrypt(mustKey(t), []byte("secret"), nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Decrypt(mustKey(t), ct, nil); err == nil {
		t.Error("Decrypt with wrong key should fail")
	}
}

func TestDecryptWrongAADFails(t *testing.T) {
	key := mustKey(t)
	ct, err := Encrypt(key, []byte("secret"), []byte(TagItemKey))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Decrypt(key, ct, []byte(TagVaultContent)); err == nil {
		t.Error("Decrypt with mismatched AAD should fail")
	}
}

func TestDecryptTamperedFails(t *testing.T) {
	key := mustKey(t)
	ct, err := Encrypt(key, []byte("secret payload"), nil)
	if err != nil {
		t.Fatal(err)
	}
	ct[len(ct)-1] ^= 0xff // flip a tag byte
	if _, err := Decrypt(key, ct, nil); err == nil {
		t.Error("Decrypt of tampered ciphertext should fail")
	}
}

func TestKeyLengthGuards(t *testing.T) {
	short := make([]byte, KeyLen-1)
	if _, err := Encrypt(short, []byte("x"), nil); err == nil {
		t.Error("Encrypt with short key should fail")
	}
	if _, err := Decrypt(short, make([]byte, IVLen+16), nil); err == nil {
		t.Error("Decrypt with short key should fail")
	}
}

func TestDecryptTooShortFails(t *testing.T) {
	key := mustKey(t)
	if _, err := Decrypt(key, make([]byte, IVLen-1), nil); err == nil {
		t.Error("Decrypt of data shorter than the IV should fail")
	}
}

func TestNewKeyUnique(t *testing.T) {
	a, b := mustKey(t), mustKey(t)
	if bytes.Equal(a, b) {
		t.Error("two NewKey calls returned identical keys")
	}
}
