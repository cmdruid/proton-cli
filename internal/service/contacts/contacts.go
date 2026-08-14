package contacts

import (
	"context"
	"fmt"
	"strings"

	"github.com/roman-16/proton-cli/internal/account/keys"
	"github.com/roman-16/proton-cli/internal/crypto/pgp"
	"github.com/roman-16/proton-cli/internal/idcache"
	"github.com/roman-16/proton-cli/internal/proton"
	"github.com/roman-16/proton-cli/internal/ref"
	"github.com/roman-16/proton-cli/internal/vcard"
)

type Service struct {
	C    proton.Doer
	keys keys.Get
}

func New(c proton.Doer, k keys.Get) *Service { return &Service{C: c, keys: k} }

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

func encryptedPart(nc NewContact) vcard.Encrypted {
	return vcard.Encrypted{
		Phones: nc.Phones, Note: nc.Note, Org: nc.Org, Title: nc.Title,
		Birthday: nc.Birthday, Address: nc.Address, URL: nc.URL,
	}
}

// signedPart builds the signed card for a set of addresses, carrying over the
// pinned keys and crypto settings any previous card held for an address that
// survives. Rebuilding from the addresses alone would silently unpin keys.
func signedPart(name, uid string, emails []string, previous *vcard.Signed) vcard.Signed {
	model := vcard.Signed{Name: name, UID: uid}
	for _, addr := range emails {
		if addr == "" {
			continue
		}
		e := vcard.SignedEmail{Address: addr}
		if previous != nil {
			if prev := previous.FindEmail(addr); prev != nil {
				e.KeyValues, e.Encrypt, e.Sign, e.Scheme = prev.KeyValues, prev.Encrypt, prev.Sign, prev.Scheme
			}
		}
		model.Emails = append(model.Emails, e)
	}
	return model
}

func (s *Service) List(ctx context.Context) ([]Contact, error) {
	var out []Contact
	for page := 0; ; page++ {
		var r struct {
			Contacts []struct {
				ID    string
				Cards []map[string]any
			}
		}
		q := proton.Query("Page", fmt.Sprintf("%d", page), "PageSize", "50")
		// The first page and the keys are asked for together; every page after it
		// finds them already there.
		u, err := s.keys.Alongside(ctx, func(ctx context.Context) error {
			return s.C.Decode(ctx, proton.Request{Method: "GET", Path: "/contacts/v4/contacts/export", Query: q}, &r)
		})
		if err != nil {
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

func (s *Service) Get(ctx context.Context, id string) (*Contact, error) {
	var r struct {
		Contact struct {
			ID    string
			Cards []map[string]any
		}
	}
	u, err := s.keys.Alongside(ctx, func(ctx context.Context) error {
		return s.C.Decode(ctx, proton.Request{Method: "GET", Path: "/contacts/v4/contacts/" + id}, &r)
	})
	if err != nil {
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

func (s *Service) Resolve(ctx context.Context, r string) (string, error) {
	if idcache.IsFullID(r) {
		return r, nil
	}
	contacts, err := s.List(ctx)
	if err != nil {
		return "", err
	}
	needle := strings.ToLower(r)
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
	c, err := ref.Pick("contact", r, matches,
		func(c Contact) string { return c.ID },
		func(c Contact) string { return fmt.Sprintf("%s <%s>", c.Name, c.Email) })
	if err != nil {
		return "", err
	}
	return c.ID, nil
}

func (s *Service) Create(ctx context.Context, nc NewContact) (string, error) {
	u, err := s.keys(ctx)
	if err != nil {
		return "", err
	}
	if nc.Name == "" && len(nc.Emails) == 0 {
		return "", fmt.Errorf("name or email is required")
	}
	name := nc.Name
	if name == "" {
		name = nc.Emails[0]
	}
	signed := vcard.BuildSigned(signedPart(name, vcard.UID(), nc.Emails, nil))
	signedCard, err := pgp.SignCard(signed, u.UserKR)
	if err != nil {
		return "", err
	}
	cards := []any{signedCard}
	if hasEncryptedFields(nc) {
		enc := vcard.BuildEncrypted(encryptedPart(nc))
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

func (s *Service) Update(ctx context.Context, id string, patch NewContact) error {
	existing, err := s.Get(ctx, id)
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
	old := vcard.ParseSigned(strings.Join(existing.Cards, "\n"))
	uid := old.UID
	if uid == "" {
		uid = vcard.UID()
	}
	name := merged.Name
	if name == "" && len(merged.Emails) > 0 {
		name = merged.Emails[0]
	}
	u, err := s.keys(ctx)
	if err != nil {
		return err
	}
	signedCard, err := pgp.SignCard(vcard.BuildSigned(signedPart(name, uid, merged.Emails, &old)), u.UserKR)
	if err != nil {
		return err
	}
	cards := []any{signedCard}
	if hasEncryptedFields(merged) {
		enc := vcard.BuildEncrypted(encryptedPart(merged))
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

func contactFromCards(id string, cards []string) Contact {
	joined := strings.Join(cards, "\n")
	emails := vcard.Values(joined, "EMAIL")
	phones := vcard.Values(joined, "TEL")
	c := Contact{
		ID:       id,
		Name:     vcard.Field(joined, "FN"),
		Emails:   emails,
		Phones:   phones,
		Org:      vcard.Field(joined, "ORG"),
		Note:     vcard.Field(joined, "NOTE"),
		Title:    vcard.Field(joined, "TITLE"),
		Birthday: vcard.Field(joined, "BDAY"),
		Address:  vcard.Field(joined, "ADR"),
		URL:      vcard.Field(joined, "URL"),
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
