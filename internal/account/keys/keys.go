// Package keys unlocks the Proton user/address/key hierarchy for a session.
package keys

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/ProtonMail/go-srp"
	pgp "github.com/ProtonMail/gopenpgp/v2/crypto"
	"github.com/cmdruid/proton-cli/internal/account/localkey"
	"github.com/cmdruid/proton-cli/internal/errs"
	"github.com/cmdruid/proton-cli/internal/fetch"
	"github.com/cmdruid/proton-cli/internal/proton"
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

// KeyPassword supplies the secret the key hierarchy is locked with: the account
// password, or - where Proton's two-password mode keeps the two apart - the
// account's second password.
//
// Which of them is wanted is the account's to say and not the command's, so it
// is passed in rather than asked about. It is called only when the session
// carries no usable key password, so a resumed session unlocks without asking
// for anything.
type KeyPassword func(twoPassword bool) (string, error)

// errWrongKeyPass reports that the user keys did not open with the passphrase
// given. It is the one unlocking failure a different secret would fix, which is
// what makes it worth telling apart from the rest.
var errWrongKeyPass = errors.New("the key password did not open the user keys")

// Unlock fetches the user and address keys and unlocks them.
//
// The key password sealed into the session, the user and the addresses are asked
// for at the same time: none of the three answers is needed to ask for another.
// Only the unlocking that follows them has an order - the user keys open the
// address keys - and only a session without a usable seal reaches the person.
func Unlock(ctx context.Context, c *proton.Client, ask KeyPassword) (*Unlocked, error) {
	var (
		sealed string
		user   *User
		addrs  []Address
	)
	if err := fetch.Together(ctx,
		func(ctx context.Context) error {
			sealed = sealedKeyPass(ctx, c)
			return nil
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

	if sealed != "" {
		u, err := open(user, addrs, sealed)
		if err == nil {
			return u, nil
		}
		if !errors.Is(err, errWrongKeyPass) {
			return nil, err
		}
		// The seal opened and what it held is stale, which is what a password
		// changed elsewhere leaves behind. Deriving again is the repair.
		slog.Debug("keys: the sealed key password no longer opens the keys; deriving a fresh one")
	}

	d, err := derive(ctx, c, user, ask)
	if err != nil {
		return nil, err
	}
	u, err := open(user, addrs, d.pass)
	if err != nil {
		if errors.Is(err, errWrongKeyPass) {
			return nil, wrongSecret(d.twoPassword)
		}
		return nil, err
	}
	wrapAndPersist(ctx, c, d.pass)
	return u, nil
}

// open unlocks the hierarchy with a passphrase: the user keys, then the address
// keys they hold the tokens for.
func open(user *User, addrs []Address, skp string) (*Unlocked, error) {
	userKR, err := unlockKeyRing(user.Keys, []byte(skp), nil)
	if err != nil {
		return nil, errWrongKeyPass
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

// wrongSecret says which of the account's secrets failed to open its keys.
//
// In two-password mode it can only be the second one: the password that signs in
// has just been proved over SRP, and saying so is the difference between a typo
// somebody can fix and a mystery.
func wrongSecret(twoPassword bool) error {
	if twoPassword {
		return errs.Problemf("Incorrect second password.")
	}
	return errs.Problemf("Your password did not unlock your keys.")
}

// sealedKeyPass recovers the key password the session carries, if it carries one
// that still opens.
//
// The blob is wrapped with a key that lives server-side, so a revoked session, a
// sign-in over the top of another and a rotated client key all leave one that
// cannot be opened. None of those is a reason to fail: the blob is a cache of
// something derivable, and a cache that cannot be read is a miss.
func sealedKeyPass(ctx context.Context, c *proton.Client) string {
	blob := c.EncKeyBlob()
	if blob == "" {
		return ""
	}
	key, err := localkey.Get(ctx, c)
	if err != nil {
		slog.Debug("localkey: the session's client key could not be fetched; deriving the key password instead", "err", err)
		return ""
	}
	skp, err := localkey.Unwrap(blob, key)
	if err != nil {
		slog.Debug("localkey: the sealed key password could not be opened; deriving one instead", "err", err)
		return ""
	}
	return skp
}

// derived is a key password worked out from a secret somebody supplied, and
// which of the account's secrets that was.
type derived struct {
	pass        string
	twoPassword bool
}

// derive stretches the account's own secret into the passphrase its keys are
// locked with.
//
// What that secret is depends on the account, and Proton's settings are what say
// so - the salt and the mode are asked for together, because the answer to
// neither is needed to ask the other.
func derive(ctx context.Context, c *proton.Client, user *User, ask KeyPassword) (derived, error) {
	var (
		salts       []salt
		twoPassword bool
	)
	if err := fetch.Together(ctx,
		func(ctx context.Context) error {
			var err error
			if salts, err = getKeySalts(ctx, c); err != nil {
				return fmt.Errorf("get key salts: %w", err)
			}
			return nil
		},
		func(ctx context.Context) error {
			var err error
			if twoPassword, err = twoPasswordMode(ctx, c); err != nil {
				return fmt.Errorf("get password mode: %w", err)
			}
			return nil
		},
	); err != nil {
		return derived{}, err
	}

	secret, err := ask(twoPassword)
	if err != nil {
		return derived{}, err
	}
	pass, err := stretch(secret, saltOf(user.Keys, salts))
	if err != nil {
		return derived{}, fmt.Errorf("derive key password: %w", err)
	}
	return derived{pass: pass, twoPassword: twoPassword}, nil
}

// stretch is bcrypt over the secret and the key's salt, which is what Proton
// locks a private key with. A key from an auth version that predates salts is
// locked with the secret itself.
func stretch(secret, keySalt string) (string, error) {
	if keySalt == "" {
		return secret, nil
	}
	raw, err := base64.StdEncoding.DecodeString(keySalt)
	if err != nil {
		return "", err
	}
	sp, err := srp.MailboxPassword([]byte(secret), raw)
	if err != nil {
		return "", err
	}
	return string(sp[len(sp)-31:]), nil
}

// saltOf returns the salt the primary user key was locked with.
//
// Proton salts every key of its own and locks the whole hierarchy with the one
// belonging to the primary, so taking whichever salt came first is a coin flip
// on an account that has more than one key. Mirrors getPrimaryKeyWithSalt in
// WebClients (packages/shared/lib/keys/keys.ts), including its tolerance of a
// key with no salt at all.
func saltOf(keys []Key, salts []salt) string {
	if len(keys) == 0 {
		return ""
	}
	for _, s := range salts {
		if s.ID == keys[0].ID {
			return s.KeySalt
		}
	}
	return ""
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

func getKeySalts(ctx context.Context, c *proton.Client) ([]salt, error) {
	var r struct{ KeySalts []salt }
	if err := c.Decode(ctx, proton.Request{Method: "GET", Path: "/core/v4/keys/salts"}, &r); err != nil {
		return nil, err
	}
	return r.KeySalts, nil
}

// PasswordModeTwo is the account setting Proton calls two-password mode: the
// password that proves who the account is, and the one that opens its keys, are
// two different secrets. Mirrors PASSWORD_MODE.TWO_PASSWORD in WebClients
// (packages/shared/lib/constants.ts).
const PasswordModeTwo = 2

// twoPasswordMode reports whether the account keeps the password that proves who
// it is apart from the one that opens its keys.
//
// Proton calls it two-password mode and reports it with the account's settings,
// which is where its own clients read it from as well - the sign-in response
// carries the same fact, but a first unlock does not always follow a sign-in, and
// one fact wants one source.
func twoPasswordMode(ctx context.Context, c *proton.Client) (bool, error) {
	var r struct {
		UserSettings struct {
			Password struct{ Mode int }
		}
	}
	if err := c.Decode(ctx, proton.Request{Method: "GET", Path: "/core/v4/settings"}, &r); err != nil {
		return false, err
	}
	return r.UserSettings.Password.Mode == PasswordModeTwo, nil
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
