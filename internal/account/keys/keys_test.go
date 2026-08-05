package keys

import (
	"testing"

	pgp "github.com/ProtonMail/gopenpgp/v2/crypto"
)

func unlocked(addrs ...Address) *Unlocked {
	u := &Unlocked{Addresses: addrs, AddrKRs: map[string]*pgp.KeyRing{}}
	for _, a := range addrs {
		if len(a.Keys) > 0 {
			u.AddrKRs[a.ID] = &pgp.KeyRing{}
		}
	}
	return u
}

var withKey = []Key{{ID: "k"}}

func TestFirstAddrSkipsAddressesWithoutKeys(t *testing.T) {
	u := unlocked(
		Address{ID: "locked", Email: "locked@example.com"},
		Address{ID: "open", Email: "open@example.com", DisplayName: "Open", Keys: withKey},
	)
	kr, addr, err := u.FirstAddr()
	if err != nil {
		t.Fatalf("FirstAddr: %v", err)
	}
	if kr == nil {
		t.Error("FirstAddr returned a nil key ring")
	}
	if addr.ID != "open" || addr.DisplayName != "Open" {
		t.Errorf("FirstAddr = %+v, want the address whose keys unlocked", addr)
	}
}

func TestFirstAddrWithoutUnlockableAddresses(t *testing.T) {
	_, _, err := unlocked(Address{ID: "locked", Email: "locked@example.com"}).FirstAddr()
	if err == nil {
		t.Error("FirstAddr with no unlockable address should fail")
	}
}

func TestPrimaryAddrPrefersProtonDomains(t *testing.T) {
	tests := []struct {
		name  string
		addrs []Address
		want  string
	}{
		{
			"custom domain first, proton.me second",
			[]Address{
				{ID: "custom", Email: "me@example.com", Keys: withKey},
				{ID: "proton", Email: "me@proton.me", Keys: withKey},
			},
			"proton",
		},
		{
			"pm.me counts as a Proton domain",
			[]Address{
				{ID: "custom", Email: "me@example.com", Keys: withKey},
				{ID: "pm", Email: "me@pm.me", Keys: withKey},
			},
			"pm",
		},
		{
			"protonmail.com counts as a Proton domain",
			[]Address{
				{ID: "custom", Email: "me@example.com", Keys: withKey},
				{ID: "legacy", Email: "me@protonmail.com", Keys: withKey},
			},
			"legacy",
		},
		{
			"falls back to the first unlockable address",
			[]Address{{ID: "custom", Email: "me@example.com", Keys: withKey}},
			"custom",
		},
		{
			"ignores a Proton address whose keys stayed locked",
			[]Address{
				{ID: "custom", Email: "me@example.com", Keys: withKey},
				{ID: "proton", Email: "me@proton.me"},
			},
			"custom",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, addr, err := unlocked(tc.addrs...).PrimaryAddr()
			if err != nil {
				t.Fatalf("PrimaryAddr: %v", err)
			}
			if addr.ID != tc.want {
				t.Errorf("PrimaryAddr = %q, want %q", addr.ID, tc.want)
			}
		})
	}
}
