package pass

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
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

	got, rotation, err := New(d, testKeys(nil)).latestItemKey(context.Background(), sk, "share", "item")
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
		if _, _, err := New(d, testKeys(nil)).latestItemKey(context.Background(), sk, "share", "item"); err == nil {
			t.Error("accepted a rotation with no matching share key")
		}
	})

	t.Run("a key wrapped to a different share key", func(t *testing.T) {
		d := &keyDoer{body: latestKeyJSON(t, shareKey, itemKey, 1)}
		sk := &shareKeys{keys: map[int][]byte{1: make([]byte, 32)}}
		if _, _, err := New(d, testKeys(nil)).latestItemKey(context.Background(), sk, "share", "item"); err == nil {
			t.Error("accepted a key it could not open")
		}
	})

	t.Run("a key that is not base64", func(t *testing.T) {
		body, _ := json.Marshal(map[string]any{"Key": map[string]any{"Key": "not base64!!", "KeyRotation": 1}})
		d := &keyDoer{body: body}
		sk := &shareKeys{keys: map[int][]byte{1: shareKey}}
		if _, _, err := New(d, testKeys(nil)).latestItemKey(context.Background(), sk, "share", "item"); err == nil {
			t.Error("accepted a key that is not base64")
		}
	})

	t.Run("a request that failed", func(t *testing.T) {
		// Discarding the error would leave the rotation at zero, which labels the
		// ciphertext with a key that cannot open it.
		d := &keyDoer{err: context.DeadlineExceeded}
		sk := &shareKeys{keys: map[int][]byte{1: shareKey}}
		if _, _, err := New(d, testKeys(nil)).latestItemKey(context.Background(), sk, "share", "item"); err == nil {
			t.Error("a failed request was treated as an answer")
		}
	})
}

// The identity fields are declared once and walked by the create path, the
// update path, the read path and the flag registration. If any of them stopped
// agreeing, an item would be writable in a field nothing could read back.
func TestIdentityFieldsRoundTripThroughTheDeclaration(t *testing.T) {
	values := make(map[string]string, len(IdentityFields))
	for i, f := range IdentityFields {
		values[f.Flag] = fmt.Sprintf("value-%d", i)
	}

	idn := buildIdentity(values)
	got := readIdentity(idn)
	if len(got) != len(IdentityFields) {
		t.Fatalf("wrote %d fields and read back %d", len(IdentityFields), len(got))
	}
	for flag, want := range values {
		if got[flag] != want {
			t.Errorf("%s round-tripped as %q, want %q", flag, got[flag], want)
		}
	}
}

// No two fields may share a flag, and none may point at the same place on the
// stored item - either would make one field silently overwrite another.
func TestIdentityFieldsAreDistinct(t *testing.T) {
	seenFlag := map[string]bool{}
	for _, f := range IdentityFields {
		if seenFlag[f.Flag] {
			t.Errorf("two identity fields both use --%s", f.Flag)
		}
		seenFlag[f.Flag] = true
		if f.Label == "" {
			t.Errorf("--%s has no label", f.Flag)
		}
	}
	// Two fields aliasing one storage slot: set each in turn and check nothing
	// else moved.
	for _, f := range IdentityFields {
		idn := buildIdentity(map[string]string{f.Flag: "only-this"})
		for _, other := range IdentityFields {
			if other.Flag == f.Flag {
				continue
			}
			if other.Get(idn) != "" {
				t.Errorf("setting --%s also set --%s; they share a storage slot",
					f.Flag, other.Flag)
			}
		}
	}
}

// A patch changes only what it names, so editing a job title leaves the address
// exactly as it was.
func TestPatchIdentityLeavesTheRestAlone(t *testing.T) {
	idn := buildIdentity(map[string]string{"first-name": "Jane", "city": "Vienna"})
	patchIdentity(idn, map[string]string{"city": "Graz"})

	got := readIdentity(idn)
	if got["city"] != "Graz" {
		t.Errorf("city = %q, want Graz", got["city"])
	}
	if got["first-name"] != "Jane" {
		t.Errorf("changing the city dropped the first name: %q", got["first-name"])
	}
}
