package pass

import (
	"encoding/base64"
	"testing"

	"github.com/roman-16/proton-cli/internal/crypto/aead"
	pb "github.com/roman-16/proton-cli/internal/service/pass/proto"
	"google.golang.org/protobuf/proto"
)

func vaultKey() []byte {
	key := make([]byte, 32)
	for i := range key {
		key[i] = byte(i)
	}
	return key
}

func storedVault(t *testing.T, key []byte, v *pb.Vault) string {
	t.Helper()
	raw, err := proto.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	ct, err := aead.Encrypt(key, raw, []byte(aead.TagVaultContent))
	if err != nil {
		t.Fatal(err)
	}
	return base64.StdEncoding.EncodeToString(ct)
}

// A rename replaces the whole stored vault, so everything it did not name has to
// survive. Rebuilding from an empty vault is how a rename loses a description.
func TestRenamedVaultKeepsEverythingElse(t *testing.T) {
	key := vaultKey()
	content := storedVault(t, key, &pb.Vault{
		Name:        "Personal",
		Description: "cards and logins",
		Display:     &pb.VaultDisplayPreferences{Icon: 3, Color: 5},
	})

	out, err := renamedVault(content, key, "Private")
	if err != nil {
		t.Fatal(err)
	}
	var got pb.Vault
	if err := proto.Unmarshal(out, &got); err != nil {
		t.Fatal(err)
	}
	if got.Name != "Private" {
		t.Errorf("name = %q", got.Name)
	}
	if got.Description != "cards and logins" {
		t.Errorf("the rename dropped the description: %q", got.Description)
	}
	if got.Display == nil || got.Display.Icon != 3 || got.Display.Color != 5 {
		t.Errorf("the rename dropped the display settings: %+v", got.Display)
	}
}

func TestRenamedVaultRefusesContentItCannotRead(t *testing.T) {
	key := vaultKey()
	other := make([]byte, 32)

	if _, err := renamedVault("", key, "Private"); err == nil {
		t.Error("a vault with no stored content was renamed anyway")
	}
	content := storedVault(t, key, &pb.Vault{Name: "Personal", Description: "keep me"})
	if _, err := renamedVault(content, other, "Private"); err == nil {
		t.Error("a vault whose content could not be decrypted was renamed anyway")
	}
	if _, err := renamedVault("not base64!!", key, "Private"); err == nil {
		t.Error("content that is not base64 was renamed anyway")
	}
}

// The rotation sent has to name the key the content was encrypted with, so the
// newest key is what a write uses.
func TestShareKeysLatestPicksTheHighestRotation(t *testing.T) {
	sk := &shareKeys{keys: map[int][]byte{1: {1}, 3: {3}, 2: {2}}}
	key, rotation := sk.latest()
	if rotation != 3 || len(key) != 1 || key[0] != 3 {
		t.Errorf("latest = %v, %d; want the key for rotation 3", key, rotation)
	}
	empty := &shareKeys{keys: map[int][]byte{}}
	if key, rotation := empty.latest(); key != nil || rotation != -1 {
		t.Errorf("latest on no keys = %v, %d", key, rotation)
	}
}
