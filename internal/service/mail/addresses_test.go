package mail

import (
	"testing"

	pgp "github.com/ProtonMail/gopenpgp/v2/crypto"
	"github.com/roman-16/proton-cli/internal/account/keys"
)

// testSender is a stand-in sending identity for tests that only need an address,
// not working crypto.
func testSender() *Sender {
	return &Sender{Address: keys.Address{
		ID: "addr-1", Email: "sender@proton.me", DisplayName: "Sender",
		Status: 1, Send: 1, Receive: 1,
	}}
}

// unlockedWith builds an Unlocked whose every address has an (empty) key ring, so
// sender selection sees them all as usable.
func unlockedWith(addrs ...keys.Address) *keys.Unlocked {
	krs := map[string]*pgp.KeyRing{}
	for _, a := range addrs {
		kr, err := pgp.NewKeyRing(nil)
		if err != nil {
			panic(err)
		}
		krs[a.ID] = kr
	}
	return &keys.Unlocked{AddrKRs: krs, Addresses: addrs}
}

func addr(id, email string, order int, opts ...func(*keys.Address)) keys.Address {
	a := keys.Address{ID: id, Email: email, Order: order, Status: 1, Send: 1, Receive: 1}
	for _, o := range opts {
		o(&a)
	}
	return a
}

func disabled(a *keys.Address)    { a.Status = 0 }
func sendOnly(a *keys.Address)    { a.Receive = 0 }
func receiveOnly(a *keys.Address) { a.Send = 0 }

func TestResolveSenderUsesAccountOrder(t *testing.T) {
	u := unlockedWith(
		addr("b", "second@proton.me", 2),
		addr("a", "first@proton.me", 1),
	)
	got, err := resolveSender(u, SenderRequest{})
	if err != nil {
		t.Fatalf("ResolveSender: %v", err)
	}
	if got.Address.Email != "first@proton.me" {
		t.Errorf("sender = %q, want the lowest-Order address", got.Address.Email)
	}
}

func TestResolveSenderSkipsAddressesThatCannotSend(t *testing.T) {
	u := unlockedWith(
		addr("a", "disabled@proton.me", 1, disabled),
		addr("b", "receive-only@proton.me", 2, receiveOnly),
		addr("c", "send-only@proton.me", 3, sendOnly),
		addr("d", "usable@proton.me", 4),
	)
	got, err := resolveSender(u, SenderRequest{})
	if err != nil {
		t.Fatalf("ResolveSender: %v", err)
	}
	if got.Address.Email != "usable@proton.me" {
		t.Errorf("sender = %q, want usable@proton.me", got.Address.Email)
	}
}

func TestResolveSenderExplicitByEmailAndID(t *testing.T) {
	u := unlockedWith(addr("a", "first@proton.me", 1), addr("b", "work@example.com", 2))
	for _, want := range []string{"work@example.com", "WORK@EXAMPLE.COM", "b"} {
		got, err := resolveSender(u, SenderRequest{Explicit: want})
		if err != nil {
			t.Fatalf("resolveSender(%q): %v", want, err)
		}
		if got.Address.ID != "b" {
			t.Errorf("resolveSender(%q) picked %q, want address b", want, got.Address.ID)
		}
	}
}

func TestResolveSenderExplicitPlusAliasKeepsTheAlias(t *testing.T) {
	u := unlockedWith(addr("a", "me@proton.me", 1))
	got, err := resolveSender(u, SenderRequest{Explicit: "me+shopping@proton.me"})
	if err != nil {
		t.Fatalf("ResolveSender: %v", err)
	}
	if got.Address.ID != "a" {
		t.Errorf("alias resolved to %q, want the base address a", got.Address.ID)
	}
	if got.Address.Email != "me+shopping@proton.me" {
		t.Errorf("email = %q, want the alias preserved", got.Address.Email)
	}
}

func TestResolveSenderUnknownExplicitIsNotFound(t *testing.T) {
	u := unlockedWith(addr("a", "me@proton.me", 1))
	_, err := resolveSender(u, SenderRequest{Explicit: "nope@elsewhere.test"})
	if err == nil {
		t.Fatal("expected an error for an address that is not on the account")
	}
	if code := exitCodeOf(err); code != 3 {
		t.Errorf("exit code = %d, want 3 (not found)", code)
	}
}

func TestResolveSenderFollowsTheParentAddress(t *testing.T) {
	u := unlockedWith(addr("a", "first@proton.me", 1), addr("b", "work@example.com", 2))

	// A reply leaves from the address the parent arrived on, not the default.
	got, err := resolveSender(u, SenderRequest{ParentAddressID: "b"})
	if err != nil {
		t.Fatalf("ResolveSender: %v", err)
	}
	if got.Address.ID != "b" {
		t.Errorf("sender = %q, want the parent's address b", got.Address.ID)
	}

	// Matching on the address the mail was sent to works too, for a parent whose
	// AddressID is no longer around.
	got, err = resolveSender(u, SenderRequest{ParentAddress: "work@example.com"})
	if err != nil {
		t.Fatalf("ResolveSender: %v", err)
	}
	if got.Address.ID != "b" {
		t.Errorf("sender = %q, want b", got.Address.ID)
	}
}

func TestResolveSenderParentPlusAliasSendsAsTheAlias(t *testing.T) {
	u := unlockedWith(addr("a", "me@proton.me", 1))
	got, err := resolveSender(u, SenderRequest{ParentAddress: "me+newsletter@proton.me", ParentAddressID: "a"})
	if err != nil {
		t.Fatalf("ResolveSender: %v", err)
	}
	if got.Address.Email != "me+newsletter@proton.me" {
		t.Errorf("email = %q, want the alias the parent arrived on", got.Address.Email)
	}
}

func TestPlusAliasBase(t *testing.T) {
	tests := []struct{ in, want string }{
		{"me+tag@proton.me", "me@proton.me"},
		{"me@proton.me", "me@proton.me"},
		{"me+a+b@proton.me", "me@proton.me"},
		{"not-an-address", ""},
		{"weird+@proton.me", "weird@proton.me"},
	}
	for _, tt := range tests {
		if got := plusAliasBase(tt.in); got != tt.want {
			t.Errorf("plusAliasBase(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

// exitCodeOf reports the exit code an error carries, or 0.
func exitCodeOf(err error) int {
	type exitCoder interface{ ExitCode() int }
	if ec, ok := err.(exitCoder); ok {
		return ec.ExitCode()
	}
	return 0
}
