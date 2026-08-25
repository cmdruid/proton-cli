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

// An edit replaces the whole stored vault, so everything it did not name has to
// survive. Rebuilding from an empty vault is how a rename loses a description.
func TestPatchedVaultKeepsEverythingElse(t *testing.T) {
	key := vaultKey()
	content := storedVault(t, key, &pb.Vault{
		Name:        "Personal",
		Description: "cards and logins",
		Display:     &pb.VaultDisplayPreferences{Icon: 3, Color: 5},
	})

	name := "Private"
	out, err := patchedVault(content, key, VaultPatch{Name: &name})
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

func TestPatchedVaultRefusesContentItCannotRead(t *testing.T) {
	key := vaultKey()
	other := make([]byte, 32)
	name := "Private"
	patch := VaultPatch{Name: &name}

	if _, err := patchedVault("", key, patch); err == nil {
		t.Error("a vault with no stored content was renamed anyway")
	}
	content := storedVault(t, key, &pb.Vault{Name: "Personal", Description: "keep me"})
	if _, err := patchedVault(content, other, patch); err == nil {
		t.Error("a vault whose content could not be decrypted was renamed anyway")
	}
	if _, err := patchedVault("not base64!!", key, patch); err == nil {
		t.Error("content that is not base64 was renamed anyway")
	}
}

// A patch changes only what it names: an icon set on a vault leaves its name and
// description exactly as they were.
func TestPatchedVaultChangesOnlyWhatItNames(t *testing.T) {
	key := vaultKey()
	content := storedVault(t, key, &pb.Vault{
		Name: "Personal", Description: "cards and logins",
		Display: &pb.VaultDisplayPreferences{Icon: 3, Color: 5},
	})

	icon := 7
	out, err := patchedVault(content, key, VaultPatch{Icon: &icon})
	if err != nil {
		t.Fatal(err)
	}
	var got pb.Vault
	if err := proto.Unmarshal(out, &got); err != nil {
		t.Fatal(err)
	}
	if got.Name != "Personal" || got.Description != "cards and logins" {
		t.Errorf("setting an icon changed something else: %+v", &got)
	}
	if got.Display.Icon != pb.VaultIcon(DisplayValue(7)) {
		t.Errorf("icon = %v, want the enum for 7", got.Display.Icon)
	}
	if got.Display.Color != 5 {
		t.Errorf("setting an icon dropped the colour: %v", got.Display.Color)
	}
}

// The numbers a person writes and the enum Pass stores are offset, and the two
// have to agree in both directions or a vault reads back as a different colour
// than it was set to.
func TestDisplayNumbersRoundTrip(t *testing.T) {
	for n := 1; n <= 30; n++ {
		if got := DisplayNumber(DisplayValue(n)); got != n {
			t.Errorf("%d round-tripped as %d", n, got)
		}
	}
	// A vault that never chose reads as nothing chosen, not as number one.
	if got := DisplayNumber(0); got != 0 {
		t.Errorf("an unset display read as %d, want 0", got)
	}
	if got := DisplayNumber(1); got != 0 {
		t.Errorf("a custom display read as %d, want 0", got)
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
