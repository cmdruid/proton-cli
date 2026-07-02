// Package contacts provides Proton Contacts operations.
package contacts

import (
	"context"
	"encoding/base64"
	"fmt"
	"strings"

	gopenpgp "github.com/ProtonMail/gopenpgp/v2/crypto"
	"github.com/roman-16/proton-cli/internal/crypto/ical"
	"github.com/roman-16/proton-cli/internal/crypto/keys"
	"github.com/roman-16/proton-cli/internal/crypto/pgp"
	"github.com/roman-16/proton-cli/internal/errs"
	"github.com/roman-16/proton-cli/internal/idcache"
	"github.com/roman-16/proton-cli/internal/proton"
)

type Service struct{ C proton.Doer }

func New(c proton.Doer) *Service { return &Service{C: c} }

type Contact struct {
	ID       string   `json:"id"`
	Name     string   `json:"name"`
	Email    string   `json:"email,omitempty"`
	Phone    string   `json:"phone,omitempty"`
	Emails   []string `json:"emails,omitempty"`
	Phones   []string `json:"phones,omitempty"`
	Org      string   `json:"org,omitempty"`
	Note     string   `json:"note,omitempty"`
	Title    string   `json:"title,omitempty"`
	Birthday string   `json:"birthday,omitempty"`
	Address  string   `json:"address,omitempty"`
	URL      string   `json:"url,omitempty"`
	Cards    []string `json:"cards,omitempty"`

	Signature pgp.VerifyResult `json:"signature,omitempty"`
}

type NewContact struct {
	Name     string
	Emails   []string
	Phones   []string
	Note     string
	Org      string
	Title    string
	Birthday string
	Address  string
	URL      string
}

func hasEncryptedFields(nc NewContact) bool {
	return len(nc.Phones) > 0 || nc.Note != "" || nc.Org != "" || nc.Title != "" ||
		nc.Birthday != "" || nc.Address != "" || nc.URL != ""
}

func toVCardFields(nc NewContact) ical.VCardFields {
	return ical.VCardFields{
		Phones: nc.Phones, Note: nc.Note, Org: nc.Org, Title: nc.Title,
		Birthday: nc.Birthday, Address: nc.Address, URL: nc.URL,
	}
}

func (s *Service) List(ctx context.Context, u *keys.Unlocked) ([]Contact, error) {
	var out []Contact
	for page := 0; ; page++ {
		var r struct {
			Contacts []struct {
				ID    string
				Cards []map[string]any
			}
		}
		q := keys.Query("Page", fmt.Sprintf("%d", page), "PageSize", "50")
		if err := s.C.Decode(ctx, proton.Request{Method: "GET", Path: "/contacts/v4/contacts/export", Query: q}, &r); err != nil {
			return nil, err
		}
		if len(r.Contacts) == 0 {
			break
		}
		for _, c := range r.Contacts {
			cards, verdicts, err := pgp.DecryptCardsRaw(c.Cards, u.UserKR, u.UserKR, nil)
			if err != nil {
				continue
			}
			ct := contactFromCards(c.ID, cards)
			ct.Signature = pgp.Aggregate(verdicts...)
			out = append(out, ct)
		}
		if len(r.Contacts) < 50 {
			break
		}
	}
	return out, nil
}

func (s *Service) Get(ctx context.Context, u *keys.Unlocked, id string) (*Contact, error) {
	var r struct {
		Contact struct {
			ID    string
			Cards []map[string]any
		}
	}
	if err := s.C.Decode(ctx, proton.Request{Method: "GET", Path: "/contacts/v4/contacts/" + id}, &r); err != nil {
		return nil, err
	}
	cards, verdicts, err := pgp.DecryptCardsRaw(r.Contact.Cards, u.UserKR, u.UserKR, nil)
	if err != nil {
		return nil, err
	}
	c := contactFromCards(r.Contact.ID, cards)
	c.Cards = cards
	c.Signature = pgp.Aggregate(verdicts...)
	return &c, nil
}

func (s *Service) Resolve(ctx context.Context, u *keys.Unlocked, ref string) (string, error) {
	if looksLikeID(ref) {
		return ref, nil
	}
	contacts, err := s.List(ctx, u)
	if err != nil {
		return "", err
	}
	needle := strings.ToLower(ref)
	var matches []Contact
	for _, c := range contacts {
		match := strings.Contains(strings.ToLower(c.Name), needle)
		for _, e := range c.Emails {
			if strings.Contains(strings.ToLower(e), needle) {
				match = true
				break
			}
		}
		if match {
			matches = append(matches, c)
		}
	}
	switch len(matches) {
	case 0:
		return "", &errs.NotFound{Kind: "contact", Ref: ref}
	case 1:
		return matches[0].ID, nil
	}
	cands := make([]errs.Candidate, 0, len(matches))
	for _, m := range matches {
		cands = append(cands, errs.Candidate{ID: m.ID, Label: fmt.Sprintf("%s <%s>", m.Name, m.Email)})
	}
	return "", &errs.Ambiguous{Kind: "contact", Ref: ref, Candidates: cands}
}

func (s *Service) Create(ctx context.Context, u *keys.Unlocked, nc NewContact) (string, error) {
	if nc.Name == "" && len(nc.Emails) == 0 {
		return "", fmt.Errorf("name or email is required")
	}
	name := nc.Name
	if name == "" {
		name = nc.Emails[0]
	}
	signed := ical.SignedVCard(name, nc.Emails, ical.ContactUID())
	signedCard, err := pgp.SignCard(signed, u.UserKR)
	if err != nil {
		return "", err
	}
	cards := []any{signedCard}
	if hasEncryptedFields(nc) {
		enc := ical.EncryptedVCard(toVCardFields(nc))
		ec, err := pgp.EncryptAndSignCard(enc, u.UserKR, u.UserKR)
		if err != nil {
			return "", err
		}
		cards = append(cards, ec)
	}
	body := map[string]any{
		"Contacts":  []map[string]any{{"Cards": cards}},
		"Overwrite": 0,
		"Labels":    0,
	}
	var r struct {
		Responses []struct {
			Response struct {
				Contact struct{ ID string }
			}
		}
	}
	if err := s.C.Decode(ctx, proton.Request{Method: "POST", Path: "/contacts/v4/contacts", Body: body}, &r); err != nil {
		return "", err
	}
	if len(r.Responses) > 0 {
		return r.Responses[0].Response.Contact.ID, nil
	}
	return "", nil
}

func (s *Service) Update(ctx context.Context, u *keys.Unlocked, id string, patch NewContact) error {
	existing, err := s.Get(ctx, u, id)
	if err != nil {
		return err
	}
	merged := NewContact{
		Name:     firstNonEmpty(patch.Name, existing.Name),
		Emails:   pickSlice(patch.Emails, existing.Emails),
		Phones:   pickSlice(patch.Phones, existing.Phones),
		Note:     firstNonEmpty(patch.Note, existing.Note),
		Org:      firstNonEmpty(patch.Org, existing.Org),
		Title:    firstNonEmpty(patch.Title, existing.Title),
		Birthday: firstNonEmpty(patch.Birthday, existing.Birthday),
		Address:  firstNonEmpty(patch.Address, existing.Address),
		URL:      firstNonEmpty(patch.URL, existing.URL),
	}
	// Preserve any pinned keys / crypto flags the existing signed card holds
	// for emails that survive the update; rebuilding from scratch would drop them.
	old := ical.ParseSignedVCard(strings.Join(existing.Cards, "\n"))
	uid := old.UID
	if uid == "" {
		uid = ical.ContactUID()
	}
	name := merged.Name
	if name == "" && len(merged.Emails) > 0 {
		name = merged.Emails[0]
	}
	model := ical.SignedContact{Name: name, UID: uid}
	for _, addr := range merged.Emails {
		se := ical.SignedEmail{Address: addr}
		if prev := old.FindEmail(addr); prev != nil {
			se.KeyValues = prev.KeyValues
			se.Encrypt = prev.Encrypt
			se.Sign = prev.Sign
			se.Scheme = prev.Scheme
		}
		model.Emails = append(model.Emails, se)
	}
	signedCard, err := pgp.SignCard(ical.BuildSignedVCard(model), u.UserKR)
	if err != nil {
		return err
	}
	cards := []any{signedCard}
	if hasEncryptedFields(merged) {
		enc := ical.EncryptedVCard(toVCardFields(merged))
		ec, err := pgp.EncryptAndSignCard(enc, u.UserKR, u.UserKR)
		if err != nil {
			return err
		}
		cards = append(cards, ec)
	}
	return s.C.Decode(ctx, proton.Request{Method: "PUT", Path: "/contacts/v4/contacts/" + id, Body: map[string]any{"Cards": cards}}, nil)
}

func (s *Service) Delete(ctx context.Context, ids []string) error {
	return s.C.Decode(ctx, proton.Request{Method: "PUT", Path: "/contacts/v4/contacts/delete", Body: map[string]any{"IDs": ids}}, nil)
}

// ContactCrypto holds a contact's pinned-key encryption preferences for one
// email address, mirroring the x-pm-* vCard properties Proton stores. A nil
// Encrypt/Sign means the flag is unset in the contact.
type ContactCrypto struct {
	ArmoredKeys       []string `json:"armored_keys"`
	Encrypt           *bool    `json:"encrypt,omitempty"`
	Sign              *bool    `json:"sign,omitempty"`
	Scheme            string   `json:"scheme,omitempty"`
	SignatureVerified bool     `json:"signature_verified"`
}

// PinnedKeysFor returns the pinned public keys and encryption preferences a
// contact stores for email, or nil when the address has no contact or no
// pinned key. It is best-effort on decrypt/decode: a hiccup returns (nil, nil)
// so it never blocks sending, but a contact-lookup API error propagates.
func (s *Service) PinnedKeysFor(ctx context.Context, u *keys.Unlocked, email string) (*ContactCrypto, error) {
	id, ok, err := s.contactIDByEmail(ctx, email)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, nil
	}
	ct, err := s.Get(ctx, u, id)
	if err != nil {
		return nil, nil
	}
	joined := strings.Join(ct.Cards, "\n")
	group := ical.EmailGroup(joined, email)
	if group == "" {
		return nil, nil
	}
	armored := decodePinnedKeys(ical.GroupValues(joined, group, "KEY"))
	if len(armored) == 0 {
		return nil, nil
	}
	cc := &ContactCrypto{
		ArmoredKeys:       armored,
		Scheme:            strings.ToLower(strings.TrimSpace(ical.GroupValue(joined, group, "X-PM-SCHEME"))),
		SignatureVerified: ct.Signature == pgp.Verified,
	}
	if v := ical.GroupValue(joined, group, "X-PM-ENCRYPT"); v != "" {
		b := parseVCardBool(v)
		cc.Encrypt = &b
	}
	if v := ical.GroupValue(joined, group, "X-PM-SIGN"); v != "" {
		b := parseVCardBool(v)
		cc.Sign = &b
	}
	return cc, nil
}

// contactIDByEmail resolves an email to its contact ID via the contact-emails
// endpoint. Defaults==1 means the contact has no per-email configuration, so
// it is treated as a miss.
func (s *Service) contactIDByEmail(ctx context.Context, email string) (string, bool, error) {
	var r struct {
		ContactEmails []struct {
			ContactID string
			Defaults  int
		}
	}
	if err := s.C.Decode(ctx, proton.Request{
		Method: "GET", Path: "/contacts/v4/contacts/emails", Query: keys.Query("Email", email),
	}, &r); err != nil {
		return "", false, err
	}
	if len(r.ContactEmails) == 0 || r.ContactEmails[0].Defaults == 1 {
		return "", false, nil
	}
	return r.ContactEmails[0].ContactID, true, nil
}

// decodePinnedKeys turns "data:application/pgp-keys;base64,<b64>" vCard KEY
// values into armored public keys, dropping any that fail to parse.
func decodePinnedKeys(values []string) []string {
	var out []string
	for _, v := range values {
		_, b64, ok := strings.Cut(v, ",")
		if !ok {
			continue
		}
		bin, err := base64.StdEncoding.DecodeString(strings.TrimSpace(b64))
		if err != nil {
			continue
		}
		key, err := gopenpgp.NewKey(bin)
		if err != nil {
			continue
		}
		armored, err := key.GetArmoredPublicKey()
		if err != nil {
			continue
		}
		out = append(out, armored)
	}
	return out
}

func parseVCardBool(v string) bool {
	return strings.EqualFold(strings.TrimSpace(v), "true")
}

type rawCard struct {
	Type      int
	Data      string
	Signature string
}

func (s *Service) rawContactCards(ctx context.Context, id string) ([]rawCard, error) {
	var r struct {
		Contact struct{ Cards []rawCard }
	}
	if err := s.C.Decode(ctx, proton.Request{Method: "GET", Path: "/contacts/v4/contacts/" + id}, &r); err != nil {
		return nil, err
	}
	return r.Contact.Cards, nil
}

// editableSignedCard fetches a contact's raw cards, verifies and parses the
// signed card into an editable model, and returns the remaining (encrypted/
// clear) cards verbatim so callers can re-attach them unchanged on PUT.
func (s *Service) editableSignedCard(ctx context.Context, u *keys.Unlocked, id string) (*ical.SignedContact, []map[string]any, error) {
	cards, err := s.rawContactCards(ctx, id)
	if err != nil {
		return nil, nil, err
	}
	var signedData string
	haveSigned := false
	var others []map[string]any
	for _, c := range cards {
		if c.Type == pgp.CardSigned && !haveSigned {
			msg := gopenpgp.NewPlainMessageFromString(c.Data)
			if v := pgp.VerifyDetachedStatus(u.UserKR, msg, c.Signature); v != pgp.Verified {
				return nil, nil, fmt.Errorf("contact signed card could not be verified; refusing to edit")
			}
			signedData = c.Data
			haveSigned = true
			continue
		}
		others = append(others, map[string]any{"Type": c.Type, "Data": c.Data, "Signature": c.Signature})
	}
	if !haveSigned {
		return nil, nil, fmt.Errorf("contact has no signed card to edit")
	}
	model := ical.ParseSignedVCard(signedData)
	if model.UID == "" {
		model.UID = ical.ContactUID()
	}
	return &model, others, nil
}

// putSignedCard re-signs the model and PUTs it alongside the preserved cards.
func (s *Service) putSignedCard(ctx context.Context, u *keys.Unlocked, id string, model ical.SignedContact, others []map[string]any) error {
	signedCard, err := pgp.SignCard(ical.BuildSignedVCard(model), u.UserKR)
	if err != nil {
		return err
	}
	cards := make([]any, 0, len(others)+1)
	cards = append(cards, signedCard)
	for _, o := range others {
		cards = append(cards, o)
	}
	return s.C.Decode(ctx, proton.Request{Method: "PUT", Path: "/contacts/v4/contacts/" + id, Body: map[string]any{"Cards": cards}}, nil)
}

// PinKey pins armoredKey to the contact for email as the preferred key. Encrypt
// and sign default to true (matching the web client's "trust key" flow) unless
// overridden. The signed card is re-signed; all other cards are preserved.
func (s *Service) PinKey(ctx context.Context, u *keys.Unlocked, id, email, armoredKey string, encrypt, sign *bool, scheme string) error {
	keyValue, err := encodePinnedKey(armoredKey)
	if err != nil {
		return err
	}
	model, others, err := s.editableSignedCard(ctx, u, id)
	if err != nil {
		return err
	}
	e := model.FindEmail(email)
	if e == nil {
		model.Emails = append(model.Emails, ical.SignedEmail{Address: email})
		e = &model.Emails[len(model.Emails)-1]
	}
	e.KeyValues = prependUnique(e.KeyValues, keyValue)
	trueVal := true
	if encrypt != nil {
		e.Encrypt = encrypt
	} else {
		e.Encrypt = &trueVal
	}
	if sign != nil {
		e.Sign = sign
	} else {
		signVal := true
		e.Sign = &signVal
	}
	if scheme != "" {
		e.Scheme = scheme
	}
	return s.putSignedCard(ctx, u, id, *model, others)
}

// UnpinKey removes all pinned keys and crypto flags a contact stores for email.
func (s *Service) UnpinKey(ctx context.Context, u *keys.Unlocked, id, email string) error {
	model, others, err := s.editableSignedCard(ctx, u, id)
	if err != nil {
		return err
	}
	e := model.FindEmail(email)
	if e == nil || len(e.KeyValues) == 0 {
		return &errs.NotFound{Kind: "pinned key", Ref: email}
	}
	e.KeyValues = nil
	e.Encrypt = nil
	e.Sign = nil
	e.Scheme = ""
	return s.putSignedCard(ctx, u, id, *model, others)
}

// encodePinnedKey converts an armored public key (or the public part of a
// private key) into a vCard KEY property value.
func encodePinnedKey(armored string) (string, error) {
	key, err := gopenpgp.NewKeyFromArmored(strings.TrimSpace(armored))
	if err != nil {
		return "", fmt.Errorf("invalid public key: %w", err)
	}
	bin, err := key.GetPublicKey()
	if err != nil {
		return "", fmt.Errorf("read public key: %w", err)
	}
	return "data:application/pgp-keys;base64," + base64.StdEncoding.EncodeToString(bin), nil
}

// prependUnique returns existing with v moved to the front (highest
// preference), dropping any duplicate of v.
func prependUnique(existing []string, v string) []string {
	out := make([]string, 0, len(existing)+1)
	out = append(out, v)
	for _, e := range existing {
		if e != v {
			out = append(out, e)
		}
	}
	return out
}

// Group is a contact group (a Type-2 label).
type Group struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Color string `json:"color"`
}

func (s *Service) GroupsList(ctx context.Context) ([]Group, error) {
	var r struct {
		Labels []struct{ ID, Name, Color string }
	}
	if err := s.C.Decode(ctx, proton.Request{Method: "GET", Path: "/core/v4/labels", Query: keys.Query("Type", "2")}, &r); err != nil {
		return nil, err
	}
	out := make([]Group, 0, len(r.Labels))
	for _, l := range r.Labels {
		out = append(out, Group{ID: l.ID, Name: l.Name, Color: l.Color})
	}
	return out, nil
}

func (s *Service) GroupCreate(ctx context.Context, name, color string) (string, error) {
	var r struct{ Label struct{ ID string } }
	if err := s.C.Decode(ctx, proton.Request{
		Method: "POST", Path: "/core/v4/labels",
		Body: map[string]any{"Name": name, "Color": color, "Type": 2},
	}, &r); err != nil {
		return "", err
	}
	return r.Label.ID, nil
}

func (s *Service) GroupDelete(ctx context.Context, id string) error {
	return s.C.Decode(ctx, proton.Request{Method: "DELETE", Path: "/core/v4/labels/" + id}, nil)
}

// GroupAdd adds contacts to a group; GroupRemove removes them. Both operate on
// whole contacts (all of a contact's emails join/leave the group).
func (s *Service) GroupAdd(ctx context.Context, groupID string, contactIDs []string) error {
	return s.C.Decode(ctx, proton.Request{
		Method: "PUT", Path: "/contacts/v4/contacts/label",
		Body: map[string]any{"LabelID": groupID, "ContactIDs": contactIDs},
	}, nil)
}

func (s *Service) GroupRemove(ctx context.Context, groupID string, contactIDs []string) error {
	return s.C.Decode(ctx, proton.Request{
		Method: "PUT", Path: "/contacts/v4/contacts/unlabel",
		Body: map[string]any{"LabelID": groupID, "ContactIDs": contactIDs},
	}, nil)
}

func contactFromCards(id string, cards []string) Contact {
	joined := strings.Join(cards, "\n")
	emails := ical.Fields(joined, "EMAIL")
	phones := ical.Fields(joined, "TEL")
	c := Contact{
		ID:       id,
		Name:     ical.Field(joined, "FN"),
		Emails:   emails,
		Phones:   phones,
		Org:      ical.Field(joined, "ORG"),
		Note:     ical.Field(joined, "NOTE"),
		Title:    ical.Field(joined, "TITLE"),
		Birthday: ical.Field(joined, "BDAY"),
		Address:  ical.Field(joined, "ADR"),
		URL:      ical.Field(joined, "URL"),
	}
	if len(emails) > 0 {
		c.Email = emails[0]
	}
	if len(phones) > 0 {
		c.Phone = phones[0]
	}
	return c
}

func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

func pickSlice(a, b []string) []string {
	if len(a) > 0 {
		return a
	}
	return b
}

func looksLikeID(s string) bool { return idcache.IsFullID(s) }
