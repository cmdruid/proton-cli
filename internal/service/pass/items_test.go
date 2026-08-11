package pass

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"testing"

	"github.com/roman-16/proton-cli/internal/crypto/aead"
	"github.com/roman-16/proton-cli/internal/proton"
)

// keyDoer answers the item-key endpoint from a fixture and records what was asked.
type keyDoer struct {
	body  []byte
	err   error
	paths []string
}

func (d *keyDoer) Do(context.Context, proton.Request) (*proton.Response, error) { return nil, nil }

func (d *keyDoer) Decode(_ context.Context, r proton.Request, out any) error {
	d.paths = append(d.paths, r.Path)
	if d.err != nil {
		return d.err
	}
	return json.Unmarshal(d.body, out)
}

func latestKeyJSON(t *testing.T, shareKey, itemKey []byte, rotation int) []byte {
	t.Helper()
	wrapped, err := aead.Encrypt(shareKey, itemKey, []byte(aead.TagItemKey))
	if err != nil {
		t.Fatal(err)
	}
	b, err := json.Marshal(map[string]any{"Key": map[string]any{
		"Key":         base64.StdEncoding.EncodeToString(wrapped),
		"KeyRotation": rotation,
	}})
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// The rotation a write sends has to name the key the content was encrypted with.
// Fetching the newest rotation but encrypting with an older key labels the
// ciphertext with a key that cannot open it.
func TestLatestItemKeyReturnsTheKeyItOpenedAndItsOwnRotation(t *testing.T) {
	shareKey := vaultKey()
	itemKey := make([]byte, 32)
	for i := range itemKey {
		itemKey[i] = byte(255 - i)
	}
	d := &keyDoer{body: latestKeyJSON(t, shareKey, itemKey, 4)}
	sk := &shareKeys{keys: map[int][]byte{2: make([]byte, 32), 4: shareKey}}

	got, rotation, err := New(d).latestItemKey(context.Background(), sk, "share", "item")
	if err != nil {
		t.Fatal(err)
	}
	if rotation != 4 {
		t.Errorf("rotation = %d, want the one the key came with", rotation)
	}
	if string(got) != string(itemKey) {
		t.Error("the key returned is not the key that was wrapped")
	}
	// Whatever the key opens has to open again with the same key, which is the
	// property the rotation is a claim about.
	ct, err := aead.Encrypt(got, []byte("content"), []byte(aead.TagItemContent))
	if err != nil {
		t.Fatal(err)
	}
	back, err := aead.Decrypt(itemKey, ct, []byte(aead.TagItemContent))
	if err != nil || string(back) != "content" {
		t.Errorf("content encrypted with the returned key does not open with it: %v", err)
	}
}

func TestLatestItemKeyRefusesWhatItCannotOpen(t *testing.T) {
	shareKey := vaultKey()
	itemKey := make([]byte, 32)

	t.Run("a rotation the share has no key for", func(t *testing.T) {
		d := &keyDoer{body: latestKeyJSON(t, shareKey, itemKey, 9)}
		sk := &shareKeys{keys: map[int][]byte{1: shareKey}}
		if _, _, err := New(d).latestItemKey(context.Background(), sk, "share", "item"); err == nil {
			t.Error("accepted a rotation with no matching share key")
		}
	})

	t.Run("a key wrapped to a different share key", func(t *testing.T) {
		d := &keyDoer{body: latestKeyJSON(t, shareKey, itemKey, 1)}
		sk := &shareKeys{keys: map[int][]byte{1: make([]byte, 32)}}
		if _, _, err := New(d).latestItemKey(context.Background(), sk, "share", "item"); err == nil {
			t.Error("accepted a key it could not open")
		}
	})

	t.Run("a key that is not base64", func(t *testing.T) {
		body, _ := json.Marshal(map[string]any{"Key": map[string]any{"Key": "not base64!!", "KeyRotation": 1}})
		d := &keyDoer{body: body}
		sk := &shareKeys{keys: map[int][]byte{1: shareKey}}
		if _, _, err := New(d).latestItemKey(context.Background(), sk, "share", "item"); err == nil {
			t.Error("accepted a key that is not base64")
		}
	})

	t.Run("a request that failed", func(t *testing.T) {
		// The error used to be discarded, which left the rotation at zero.
		d := &keyDoer{err: context.DeadlineExceeded}
		sk := &shareKeys{keys: map[int][]byte{1: shareKey}}
		if _, _, err := New(d).latestItemKey(context.Background(), sk, "share", "item"); err == nil {
			t.Error("a failed request was treated as an answer")
		}
	})
}
