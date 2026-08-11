// Package vcard models the contact cards Proton Contacts stores.
//
// Proton splits a contact across a signed card, which holds the name and the
// email addresses so the server can index them, and an encrypted card holding
// everything else. Per-email settings - a pinned key, whether to encrypt or sign
// to that address - hang off the signed card under a vCard group, which is why
// reading and rebuilding a contact has to preserve groups rather than flatten
// them.
package vcard

import (
	"crypto/rand"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/roman-16/proton-cli/internal/contentline"
)

// UID mints an identifier for a new contact, from a cryptographic source so that
// two contacts created in the same moment cannot collide.
func UID() string { return "proton-cli-" + rand.Text() }

// Field returns the first value of a property, ignoring any group it sits under.
func Field(text, name string) string {
	if vs := Values(text, name); len(vs) > 0 {
		return vs[0]
	}
	return ""
}

// Values returns every value of a property, in document order.
func Values(text, name string) []string {
	name = strings.ToUpper(name)
	var out []string
	for _, l := range contentline.ParseAll(text) {
		if l.Name == name {
			out = append(out, contentline.UnescapeText(l.Value))
		}
	}
	return out
}

// EmailGroup returns the group (for example "item1") whose EMAIL property matches
// email, or "". Proton stores each address's key settings under the same group as
// the address.
func EmailGroup(text, email string) string {
	want := canonical(email)
	for _, l := range contentline.ParseAll(text) {
		if l.Name == "EMAIL" && l.Group != "" && canonical(l.Value) == want {
			return l.Group
		}
	}
	return ""
}

// GroupValues returns every value of a property within one group, ordered by the
// vCard PREF parameter. Properties without a preference keep document order,
// after those that have one.
func GroupValues(text, group, field string) []string {
	field = strings.ToUpper(field)
	type ranked struct {
		pref  int
		value string
	}
	var found []ranked
	for i, l := range contentline.ParseAll(text) {
		if l.Group != group || l.Name != field {
			continue
		}
		found = append(found, ranked{pref: pref(l.Params, i), value: l.Value})
	}
	sort.SliceStable(found, func(a, b int) bool { return found[a].pref < found[b].pref })
	out := make([]string, len(found))
	for i, f := range found {
		out[i] = f.value
	}
	return out
}

// GroupValue returns the most-preferred value of a property within one group.
func GroupValue(text, group, field string) string {
	if vs := GroupValues(text, group, field); len(vs) > 0 {
		return vs[0]
	}
	return ""
}

// pref reads the PREF parameter, falling back to a large number that preserves
// document order among properties that do not declare one.
func pref(params contentline.Params, docIndex int) int {
	if v := params.Get("PREF"); v != "" {
		if n, err := strconv.Atoi(strings.TrimSpace(v)); err == nil {
			return n
		}
	}
	return 1_000_000 + docIndex
}

func canonical(email string) string {
	return strings.ToLower(strings.TrimSpace(strings.TrimPrefix(email, "mailto:")))
}

// SignedEmail is one address in the signed card, with the pinned keys and
// per-address crypto settings stored under its group.
type SignedEmail struct {
	Address   string
	KeyValues []string // raw KEY property values, in preference order
	Encrypt   *bool
	Sign      *bool
	Scheme    string
}

// Signed is the part of a contact that Proton signs but does not encrypt.
type Signed struct {
	Name   string
	UID    string
	Emails []SignedEmail
}

// FindEmail returns the entry for addr, or nil.
func (c *Signed) FindEmail(addr string) *SignedEmail {
	want := canonical(addr)
	for i := range c.Emails {
		if canonical(c.Emails[i].Address) == want {
			return &c.Emails[i]
		}
	}
	return nil
}

// ParseSigned reads a signed card, capturing each address's group so its pinned
// keys and settings survive a rebuild.
func ParseSigned(text string) Signed {
	out := Signed{Name: Field(text, "FN"), UID: Field(text, "UID")}
	seen := map[string]bool{}
	for _, l := range contentline.ParseAll(text) {
		if l.Name != "EMAIL" || l.Group == "" || seen[l.Group] {
			continue
		}
		seen[l.Group] = true
		e := SignedEmail{
			Address:   l.Value,
			KeyValues: GroupValues(text, l.Group, "KEY"),
			Scheme:    GroupValue(text, l.Group, "X-PM-SCHEME"),
		}
		if v := GroupValue(text, l.Group, "X-PM-ENCRYPT"); v != "" {
			b := strings.EqualFold(strings.TrimSpace(v), "true")
			e.Encrypt = &b
		}
		if v := GroupValue(text, l.Group, "X-PM-SIGN"); v != "" {
			b := strings.EqualFold(strings.TrimSpace(v), "true")
			e.Sign = &b
		}
		out.Emails = append(out.Emails, e)
	}
	return out
}

// BuildSigned renders a signed card, grouping each address's properties as
// item1..itemN.
func BuildSigned(c Signed) string {
	lines := []contentline.Line{
		{Name: "BEGIN", Value: "VCARD"},
		{Name: "VERSION", Value: "4.0"},
		{Name: "FN", Value: contentline.EscapeText(c.Name)},
		{Name: "UID", Value: c.UID},
	}
	n := 0
	for _, e := range c.Emails {
		if e.Address == "" {
			continue
		}
		n++
		group := fmt.Sprintf("item%d", n)
		lines = append(lines, contentline.Line{
			Group: group, Name: "EMAIL",
			Params: contentline.Params{{Name: "PREF", Value: strconv.Itoa(n)}},
			Value:  e.Address,
		})
		for i, kv := range e.KeyValues {
			lines = append(lines, contentline.Line{
				Group: group, Name: "KEY",
				Params: contentline.Params{{Name: "PREF", Value: strconv.Itoa(i + 1)}},
				Value:  kv,
			})
		}
		if e.Encrypt != nil {
			lines = append(lines, contentline.Line{Group: group, Name: "X-PM-ENCRYPT", Value: boolText(*e.Encrypt)})
		}
		if e.Sign != nil {
			lines = append(lines, contentline.Line{Group: group, Name: "X-PM-SIGN", Value: boolText(*e.Sign)})
		}
		if e.Scheme != "" {
			lines = append(lines, contentline.Line{Group: group, Name: "X-PM-SCHEME", Value: e.Scheme})
		}
	}
	lines = append(lines, contentline.Line{Name: "END", Value: "VCARD"})
	return contentline.Render(lines)
}

func boolText(b bool) string {
	if b {
		return "true"
	}
	return "false"
}

// Encrypted is the part of a contact Proton encrypts.
type Encrypted struct {
	Phones   []string
	Note     string
	Org      string
	Title    string
	Birthday string
	Address  string
	URL      string
}

// BuildEncrypted renders the encrypted card. Empty properties are left out.
func BuildEncrypted(f Encrypted) string {
	lines := []contentline.Line{
		{Name: "BEGIN", Value: "VCARD"},
		{Name: "VERSION", Value: "4.0"},
	}
	n := 0
	for _, p := range f.Phones {
		if p == "" {
			continue
		}
		n++
		lines = append(lines, contentline.Line{
			Name:   "TEL",
			Params: contentline.Params{{Name: "PREF", Value: strconv.Itoa(n)}},
			Value:  contentline.EscapeText(p),
		})
	}
	for _, kv := range []struct{ name, value string }{
		{"NOTE", f.Note},
		{"ORG", f.Org},
		{"TITLE", f.Title},
		{"BDAY", f.Birthday},
		{"ADR", f.Address},
		{"URL", f.URL},
	} {
		if kv.value != "" {
			lines = append(lines, contentline.Line{Name: kv.name, Value: contentline.EscapeText(kv.value)})
		}
	}
	lines = append(lines, contentline.Line{Name: "END", Value: "VCARD"})
	return contentline.Render(lines)
}
