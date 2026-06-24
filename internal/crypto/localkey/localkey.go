// Package localkey wraps the salted key password with a random "client key"
// that is stored server-side via /auth/v4/sessions/local/key. The encrypted
// blob is the only key material written to the session file; the client key
// never touches disk. Because the client key can only be fetched over an
// authenticated request, revoking the session makes the on-disk blob
// permanently undecryptable.
//
// This mirrors the Proton WebClients persisted-session model
// (packages/shared/lib/authentication/{clientKey,persistedSessionHelper}.ts).
package localkey

import (
	"context"
	"encoding/base64"
	"fmt"

	"github.com/roman-16/proton-cli/internal/crypto/aead"
	"github.com/roman-16/proton-cli/internal/proton"
)

const localKeyPath = "/auth/v4/sessions/local/key"

// aad is the AES-GCM additional-authenticated-data tag for domain separation.
// The blob is read only by us, so the exact value just needs to be stable.
const aad = "proton-cli.session-key"

// Generate returns a fresh random 256-bit client key.
func Generate() ([]byte, error) { return aead.NewKey() }

// Put stores the client key server-side for the current session.
func Put(ctx context.Context, c proton.Doer, key []byte) error {
	return c.Decode(ctx, proton.Request{
		Method: "PUT", Path: localKeyPath,
		Body: map[string]any{"Key": base64.StdEncoding.EncodeToString(key)},
	}, nil)
}

// Get fetches the client key the server holds for the current session. A failed
// fetch (e.g. a revoked session returning 401) means the blob cannot be
// decrypted - the property that makes revocation effective.
func Get(ctx context.Context, c proton.Doer) ([]byte, error) {
	var r struct{ ClientKey string }
	if err := c.Decode(ctx, proton.Request{Method: "GET", Path: localKeyPath}, &r); err != nil {
		return nil, err
	}
	if r.ClientKey == "" {
		return nil, fmt.Errorf("server returned no client key")
	}
	key, err := base64.StdEncoding.DecodeString(r.ClientKey)
	if err != nil {
		return nil, fmt.Errorf("decode client key: %w", err)
	}
	return key, nil
}

// Wrap encrypts the salted key password with the client key and returns a
// base64 blob suitable for the session file.
func Wrap(saltedKeyPass string, key []byte) (string, error) {
	ct, err := aead.Encrypt(key, []byte(saltedKeyPass), []byte(aad))
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(ct), nil
}

// Unwrap reverses Wrap, recovering the salted key password from the blob.
func Unwrap(blob string, key []byte) (string, error) {
	raw, err := base64.StdEncoding.DecodeString(blob)
	if err != nil {
		return "", fmt.Errorf("decode blob: %w", err)
	}
	pt, err := aead.Decrypt(key, raw, []byte(aad))
	if err != nil {
		return "", err
	}
	return string(pt), nil
}
