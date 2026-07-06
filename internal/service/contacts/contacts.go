package contacts

import (
	"context"
	"fmt"
	"strings"

	"github.com/roman-16/proton-cli/internal/account/keys"
	"github.com/roman-16/proton-cli/internal/crypto/pgp"
	"github.com/roman-16/proton-cli/internal/ical"
	"github.com/roman-16/proton-cli/internal/idcache"
	"github.com/roman-16/proton-cli/internal/proton"
	"github.com/roman-16/proton-cli/internal/ref"
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
		q := proton.Query("Page", fmt.Sprintf("%d", page), "PageSize", "50")
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

func (s *Service) Resolve(ctx context.Context, u *keys.Unlocked, r string) (string, error) {
	if idcache.IsFullID(r) {
		return r, nil
	}
	contacts, err := s.List(ctx, u)
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
