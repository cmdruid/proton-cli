// Package keys unlocks the Proton user/address/key hierarchy for a session.
package keys

import (
	"context"
	"encoding/base64"
	"fmt"
	"log/slog"
	"strings"

	"github.com/ProtonMail/go-srp"
	pgp "github.com/ProtonMail/gopenpgp/v2/crypto"
	"github.com/roman-16/proton-cli/internal/account/localkey"
	"github.com/roman-16/proton-cli/internal/fetch"
	"github.com/roman-16/proton-cli/internal/proton"
)

type Unlocked struct {
	UserKR    *pgp.KeyRing
	AddrKRs   map[string]*pgp.KeyRing
	Addresses []Address
}

// Get hands back the unlocked hierarchy, fetching it the first time it is asked
// for and remembering it after.
//
// It is a function rather than a value because keys are one more thing to fetch
// rather than a state to be in: a command that never decrypts should never pay
// for them, and one that does should be able to ask for them at the same time as
// everything else it needs instead of before it.
type Get func(context.Context) (*Unlocked, error)

// Alongside fetches the hierarchy at the same time as the caller's own request.
//
// Nothing about the keys depends on what is being fetched and nothing about the
// fetch depends on the keys, so a command that decrypts pays for them in time it
// was spending anyway. Both are always waited for here, which is what keeps the
// password prompt a first unlock may raise inside the command that asked for it.
func (get Get) Alongside(ctx context.Context, request func(context.Context) error) (*Unlocked, error) {
	var u *Unlocked
	err := fetch.Together(ctx,
		request,
		func(ctx context.Context) error {
			var err error
			u, err = get(ctx)
			return err
		},
	)
	return u, err
}

type User struct {
	ID   string
	Name string
	Keys []Key
}

// Address mirrors the fields of /core/v4/addresses the CLI needs. Order,
// Status, Send and Receive drive sender selection: Proton only allows sending
// from an address that is active and both sendable and receivable, and presents
// them in Order.
type Address struct {
	ID          string
	Email       string
	DisplayName string
	Signature   string
	Order       int
	Status      int
	Send        int
	Receive     int
	Type        int
	Keys        []Key
}

// CanSend reports whether Proton permits composing from this address.
func (a Address) CanSend() bool {
	return a.Status == 1 && a.Send == 1 && a.Receive == 1
}

type Key struct {
	ID         string
	PrivateKey string
	Token      string
	Signature  string
	Primary    int
	Active     int
}

type salt struct {
	ID      string
	KeySalt string
}

// PasswordFunc supplies the account password. It is called only when the session
// carries no sealed key password, so a resumed session unlocks without asking
// for anything.
type PasswordFunc func() (string, error)

// Unlock fetches the user and address keys and unlocks them, using either the
// key password sealed into the session or, on a first unlock, one derived from
// the account password.
//
// The key password, the user and the addresses are asked for at the same time.
// None of the three answers is needed to ask for another, and only the unlocking
// that follows them has an order: the user keys open the address keys.
func Unlock(ctx context.Context, c *proton.Client, password PasswordFunc) (*Unlocked, error) {
	var (
		skp   string
		user  *User
		addrs []Address
	)
	if err := fetch.Together(ctx,
		func(ctx context.Context) error {
			var err error
			skp, err = saltedKeyPass(ctx, c, password)
			return err
		},
		func(ctx context.Context) error {
			var err error
			if user, err = getUser(ctx, c); err != nil {
				return fmt.Errorf("get user: %w", err)
			}
			return nil
		},
		func(ctx context.Context) error {
			var err error
			if addrs, err = getAddresses(ctx, c); err != nil {
				return fmt.Errorf("get addresses: %w", err)
			}
			return nil
		},
	); err != nil {
		return nil, err
	}

	userKR, err := unlockKeyRing(user.Keys, []byte(skp), nil)
	if err != nil {
		return nil, fmt.Errorf("unlock user keys: %w", err)
	}

	addrKRs := map[string]*pgp.KeyRing{}
	for _, a := range addrs {
		if kr, err := unlockKeyRing(a.Keys, []byte(skp), userKR); err == nil {
			addrKRs[a.ID] = kr
		}
	}
	if len(addrKRs) == 0 {
		return nil, fmt.Errorf("failed to unlock any address keys")
	}
	return &Unlocked{UserKR: userKR, AddrKRs: addrKRs, Addresses: addrs}, nil
}

// saltedKeyPass is the passphrase the key hierarchy is locked with.
//
// A resumed session unwraps it from the blob on disk with the server-held client
// key, which is what makes revoking the session render that blob useless. A first
// unlock derives it from the account password, then seals and persists it so no
// later run has to ask for anything.
func saltedKeyPass(ctx context.Context, c *proton.Client, password PasswordFunc) (string, error) {
	if c.EncKeyBlob() != "" {
		key, err := localkey.Get(ctx, c)
		if err != nil {
			return "", fmt.Errorf("fetch session key: %w", err)
		}
		skp, err := localkey.Unwrap(c.EncKeyBlob(), key)
		if err != nil {
			return "", fmt.Errorf("decrypt session key blob: %w", err)
		}
		return skp, nil
	}
	if password == nil {
		return "", fmt.Errorf("no password available to unlock the keys")
	}
	pw, err := password()
	if err != nil {
		return "", err
	}
	skp, err := deriveSaltedKeyPass(ctx, c, pw)
	if err != nil {
		return "", fmt.Errorf("derive key password: %w", err)
	}
	wrapAndPersist(ctx, c, skp)
	return skp, nil
}

// wrapAndPersist generates a fresh client key, stores it server-side, wraps the
// salted key password with it, and rewrites the session file in encrypted form.
// Best-effort: skp is already held in memory for this run, so a failure here
// never fails the unlock - it only defers persistence to the next run.
func wrapAndPersist(ctx context.Context, c *proton.Client, skp string) {
	key, err := localkey.Generate()
	if err != nil {
		slog.Debug("localkey: generate failed; key password not persisted this run", "err", err)
		return
	}
	if err := localkey.Put(ctx, c, key); err != nil {
		slog.Debug("localkey: put failed; key password not persisted this run", "err", err)
		return
	}
	blob, err := localkey.Wrap(skp, key)
	if err != nil {
		slog.Debug("localkey: wrap failed; key password not persisted this run", "err", err)
		return
	}
	c.SetEncKeyBlob(blob)
	c.Persist()
}

// PrimaryAddr returns the key ring and address record for the user's primary
// proton.me/pm.me address, falling back to the first unlockable address.
func (u *Unlocked) PrimaryAddr() (*pgp.KeyRing, Address, error) {
	for _, a := range u.Addresses {
		if kr, ok := u.AddrKRs[a.ID]; ok {
			e := a.Email
			if strings.HasSuffix(e, "@proton.me") || strings.HasSuffix(e, "@pm.me") || strings.HasSuffix(e, "@protonmail.com") {
				return kr, a, nil
			}
		}
	}
	return u.FirstAddr()
}

// FirstAddr returns the key ring and address record of the first address whose
// keys could be unlocked.
func (u *Unlocked) FirstAddr() (*pgp.KeyRing, Address, error) {
	for _, a := range u.Addresses {
		if kr, ok := u.AddrKRs[a.ID]; ok {
			return kr, a, nil
		}
	}
	return nil, Address{}, fmt.Errorf("no address key rings available")
}

func (u *Unlocked) AddrKR(addrID string) (*pgp.KeyRing, bool) {
	kr, ok := u.AddrKRs[addrID]
	return kr, ok
}

func deriveSaltedKeyPass(ctx context.Context, c *proton.Client, password string) (string, error) {
	var r struct{ KeySalts []salt }
	if err := c.Decode(ctx, proton.Request{Method: "GET", Path: "/core/v4/keys/salts"}, &r); err != nil {
		return "", err
	}
	if len(r.KeySalts) == 0 {
		return "", fmt.Errorf("no key salts returned")
	}
	ks, err := base64.StdEncoding.DecodeString(r.KeySalts[0].KeySalt)
	if err != nil {
		return "", err
	}
	sp, err := srp.MailboxPassword([]byte(password), ks)
	if err != nil {
		return "", err
	}
	return string(sp[len(sp)-31:]), nil
}

func getUser(ctx context.Context, c *proton.Client) (*User, error) {
	var r struct{ User User }
	if err := c.Decode(ctx, proton.Request{Method: "GET", Path: "/core/v4/users"}, &r); err != nil {
		return nil, err
	}
	return &r.User, nil
}

func getAddresses(ctx context.Context, c *proton.Client) ([]Address, error) {
	var r struct{ Addresses []Address }
	if err := c.Decode(ctx, proton.Request{Method: "GET", Path: "/core/v4/addresses"}, &r); err != nil {
		return nil, err
	}
	return r.Addresses, nil
}

func unlockKeyRing(keys []Key, passphrase []byte, userKR *pgp.KeyRing) (*pgp.KeyRing, error) {
	kr, err := pgp.NewKeyRing(nil)
	if err != nil {
		return nil, err
	}
	for _, k := range keys {
		if k.Active == 0 {
			continue
		}
		secret := passphrase
		if k.Token != "" && k.Signature != "" && userKR != nil {
			s, err := decryptToken(k.Token, k.Signature, userKR)
			if err != nil {
				continue
			}
			secret = s
		}
		locked, err := pgp.NewKeyFromArmored(k.PrivateKey)
		if err != nil {
			continue
		}
		unlocked, err := locked.Unlock(secret)
		if err != nil {
			continue
		}
		_ = kr.AddKey(unlocked)
	}
	if kr.CountEntities() == 0 {
		return nil, fmt.Errorf("no keys could be unlocked")
	}
	return kr, nil
}

func decryptToken(tokenArm, sigArm string, kr *pgp.KeyRing) ([]byte, error) {
	msg, err := pgp.NewPGPMessageFromArmored(tokenArm)
	if err != nil {
		return nil, err
	}
	sig, err := pgp.NewPGPSignatureFromArmored(sigArm)
	if err != nil {
		return nil, err
	}
	dec, err := kr.Decrypt(msg, nil, 0)
	if err != nil {
		return nil, err
	}
	if err := kr.VerifyDetached(dec, sig, 0); err != nil {
		return nil, err
	}
	return dec.GetBinary(), nil
}

// Published returns the keys Proton holds for somebody else's address.
//
// Handing a person something encrypted - a calendar's passphrase, a vault's key -
// means encrypting it to the key Proton publishes for them. An address Proton
// holds none for is one nothing can be shared with, which is what makes this the
// place that decides whether a share is possible at all.
//
// Only the primary key is returned: it is what Proton's own clients encrypt to,
// and adding the others would mean sealing a secret under keys the owner may
// have retired.
func Published(ctx context.Context, c proton.Doer, email string) (*pgp.KeyRing, error) {
	var r struct {
		Address struct {
			Keys []struct {
				PublicKey string
				Primary   int
			}
		}
	}
	q := proton.Query()
	q.Set("Email", email)
	// Internal only: an address outside Proton has no key here, and asking for
	// external ones would return records that cannot be encrypted to.
	q.Set("InternalOnly", "1")
	if err := c.Decode(ctx, proton.Request{
		Method: "GET", Path: "/core/v4/keys/all", Query: q,
	}, &r); err != nil {
		return nil, err
	}

	kr, err := pgp.NewKeyRing(nil)
	if err != nil {
		return nil, err
	}
	for _, k := range r.Address.Keys {
		if k.Primary != 1 {
			continue
		}
		key, err := pgp.NewKeyFromArmored(k.PublicKey)
		if err != nil {
			continue
		}
		_ = kr.AddKey(key)
	}
	if len(kr.GetKeys()) == 0 {
		return nil, nil
	}
	return kr, nil
}
