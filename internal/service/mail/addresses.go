package mail

import (
	"context"
	"fmt"
	"sort"
	"strings"

	pgp "github.com/ProtonMail/gopenpgp/v2/crypto"
	"github.com/roman-16/proton-cli/internal/account/keys"
	"github.com/roman-16/proton-cli/internal/errs"
	"github.com/roman-16/proton-cli/internal/idcache"
	"github.com/roman-16/proton-cli/internal/proton"
	"github.com/roman-16/proton-cli/internal/ref"
)

// Address is one of the account's own email addresses - Proton's "Identity and
// addresses" settings page. Signature is stored as HTML.
type Address struct {
	ID          string `json:"id"`
	Email       string `json:"email"`
	DisplayName string `json:"display_name"`
	Signature   string `json:"signature,omitempty"`
	Type        int    `json:"type"`
	Status      int    `json:"status"`
	Order       int    `json:"order"`
	Send        int    `json:"send"`
	Receive     int    `json:"receive"`
}

// CanSend reports whether Proton permits composing from this address.
func (a Address) CanSend() bool { return a.Status == 1 && a.Send == 1 && a.Receive == 1 }

// rawAddress mirrors Address field for field, in Proton's own capitalisation.
// It exists because Address carries snake_case JSON tags for CLI output, which
// the decoder would otherwise fail to match against the API's DisplayName.
type rawAddress struct {
	ID          string
	Email       string
	DisplayName string
	Signature   string
	Type        int
	Status      int
	Order       int
	Send        int
	Receive     int
}

func (s *Service) AddressesList(ctx context.Context) ([]Address, error) {
	var r struct{ Addresses []rawAddress }
	if err := s.C.Decode(ctx, proton.Request{Method: "GET", Path: "/core/v4/addresses"}, &r); err != nil {
		return nil, err
	}
	out := make([]Address, 0, len(r.Addresses))
	for _, a := range r.Addresses {
		out = append(out, Address(a))
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Order < out[j].Order })
	return out, nil
}

// ResolveAddress accepts an address ID or an email address (exactly, or as the
// base of a plus alias), so every command that names one of your own addresses
// takes the form you already know.
func (s *Service) ResolveAddress(ctx context.Context, r string) (*Address, error) {
	addrs, err := s.AddressesList(ctx)
	if err != nil {
		return nil, err
	}
	if idcache.IsFullID(r) {
		for _, a := range addrs {
			if a.ID == r {
				return &a, nil
			}
		}
	}
	for _, a := range addrs {
		if strings.EqualFold(a.Email, r) || strings.EqualFold(a.Email, plusAliasBase(r)) {
			return &a, nil
		}
	}
	picked, err := ref.Pick("address", r, addrs,
		func(a Address) string { return a.ID },
		func(a Address) string { return a.Email })
	if err != nil {
		return nil, err
	}
	return &picked, nil
}

// AddressUpdate writes the display name and signature Proton shows on outgoing
// mail. A nil field is left untouched; a non-nil empty string clears it.
func (s *Service) AddressUpdate(ctx context.Context, id string, displayName, signature *string) error {
	body := map[string]any{}
	if displayName != nil {
		body["DisplayName"] = *displayName
	}
	if signature != nil {
		body["Signature"] = *signature
	}
	if len(body) == 0 {
		return fmt.Errorf("nothing to update")
	}
	return s.C.Decode(ctx, proton.Request{Method: "PUT", Path: "/core/v4/addresses/" + id, Body: body}, nil)
}

// ── sender selection ──

// Sender is a resolved sending identity: which address a message goes out from,
// and the unlocked key ring that signs and encrypts it.
type Sender struct {
	Address keys.Address
	KR      *pgp.KeyRing
}

// SenderRequest describes how to pick a sending address. Explicit is the user's
// --from, if any. ParentAddress and ParentAddressID come from the message being
// replied to or forwarded, so a reply leaves from the address that received it.
type SenderRequest struct {
	Explicit        string
	ParentAddress   string
	ParentAddressID string
}

// ResolveSender mirrors the web client's getFromAddresses/getFromAddress: only
// active, sendable, receivable addresses can compose, they are ordered by the
// account's own Order, and a plus alias resolves through its base address so a
// reply to "me+tag@proton.me" leaves from that alias.
func (s *Service) ResolveSender(ctx context.Context, req SenderRequest) (*Sender, error) {
	u, err := s.keys(ctx)
	if err != nil {
		return nil, err
	}
	return resolveSender(u, req)
}

func resolveSender(u *keys.Unlocked, req SenderRequest) (*Sender, error) {
	sendable := make([]keys.Address, 0, len(u.Addresses))
	for _, a := range u.Addresses {
		if _, ok := u.AddrKR(a.ID); ok && a.CanSend() {
			sendable = append(sendable, a)
		}
	}
	sort.SliceStable(sendable, func(i, j int) bool { return sendable[i].Order < sendable[j].Order })
	if len(sendable) == 0 {
		// Every address is disabled or its keys would not unlock; fall back to
		// whatever did unlock so single-address edge cases still send.
		kr, addr, err := u.FirstAddr()
		if err != nil {
			return nil, err
		}
		if req.Explicit != "" {
			return nil, unknownSender(req.Explicit, []keys.Address{addr})
		}
		return &Sender{Address: addr, KR: kr}, nil
	}

	if req.Explicit != "" {
		picked, err := matchSender(sendable, req.Explicit)
		if err != nil {
			return nil, err
		}
		return withKeyRing(u, *picked)
	}
	if alias := aliasOf(sendable, req.ParentAddress); alias != nil {
		return withKeyRing(u, *alias)
	}
	for _, a := range sendable {
		if a.ID == req.ParentAddressID {
			return withKeyRing(u, a)
		}
	}
	for _, a := range sendable {
		if strings.EqualFold(a.Email, req.ParentAddress) {
			return withKeyRing(u, a)
		}
	}
	return withKeyRing(u, sendable[0])
}

// matchSender resolves --from against an address ID or email, accepting a plus
// alias of one of the account's addresses.
func matchSender(sendable []keys.Address, want string) (*keys.Address, error) {
	for _, a := range sendable {
		if a.ID == want || strings.EqualFold(a.Email, want) {
			return &a, nil
		}
	}
	if alias := aliasOf(sendable, want); alias != nil {
		return alias, nil
	}
	return nil, unknownSender(want, sendable)
}

// aliasOf returns the address behind a plus alias ("me+tag@x" -> "me@x"), with
// Email rewritten to the alias so recipients see the address that was used.
func aliasOf(sendable []keys.Address, email string) *keys.Address {
	base := plusAliasBase(email)
	if base == "" || strings.EqualFold(base, email) {
		return nil
	}
	for _, a := range sendable {
		if strings.EqualFold(a.Email, base) {
			alias := a
			alias.Email = email
			return &alias
		}
	}
	return nil
}

// plusAliasBase strips a "+tag" from the local part, returning "" when the input
// is not an address.
func plusAliasBase(email string) string {
	at := strings.LastIndex(email, "@")
	if at < 0 {
		return ""
	}
	plus := strings.Index(email[:at], "+")
	if plus < 0 {
		return email
	}
	return email[:plus] + email[at:]
}

func withKeyRing(u *keys.Unlocked, a keys.Address) (*Sender, error) {
	kr, ok := u.AddrKR(a.ID)
	if !ok {
		return nil, fmt.Errorf("no unlocked key for address %s", a.Email)
	}
	return &Sender{Address: a, KR: kr}, nil
}

// unknownSender reports a --from that matches none of the account's sendable
// addresses, listing the ones that would have worked.
func unknownSender(want string, sendable []keys.Address) error {
	emails := make([]string, 0, len(sendable))
	for _, a := range sendable {
		emails = append(emails, a.Email)
	}
	return errs.WithExit(3, fmt.Errorf("no address matching %q can send mail; available: %s",
		want, strings.Join(emails, ", ")))
}
