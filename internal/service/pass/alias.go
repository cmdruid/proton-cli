package pass

import (
	"context"
	"encoding/base64"
	"fmt"
	"strings"

	"github.com/roman-16/proton-cli/internal/account/keys"
	"github.com/roman-16/proton-cli/internal/crypto/aead"
	"github.com/roman-16/proton-cli/internal/proton"
	pb "github.com/roman-16/proton-cli/internal/service/pass/proto"
	"google.golang.org/protobuf/proto"
)

type AliasSuffix struct {
	Suffix, SignedSuffix, Domain string
	IsPremium, IsCustom          bool
}

type AliasMailbox struct {
	ID    int
	Email string
}

func (s *Service) AliasOptions(ctx context.Context, shareID string) ([]AliasSuffix, []AliasMailbox, error) {
	var r struct {
		Options struct {
			Suffixes []struct {
				Suffix, SignedSuffix, Domain string
				IsPremium, IsCustom          bool
			}
			Mailboxes []struct {
				ID    int
				Email string
			}
		}
	}
	if err := s.C.Decode(ctx, proton.Request{Method: "GET", Path: "/pass/v1/share/" + shareID + "/alias/options"}, &r); err != nil {
		return nil, nil, err
	}
	var sx []AliasSuffix
	for _, x := range r.Options.Suffixes {
		sx = append(sx, AliasSuffix{Suffix: x.Suffix, SignedSuffix: x.SignedSuffix, Domain: x.Domain, IsPremium: x.IsPremium, IsCustom: x.IsCustom})
	}
	var mx []AliasMailbox
	for _, m := range r.Options.Mailboxes {
		mx = append(mx, AliasMailbox{ID: m.ID, Email: m.Email})
	}
	return sx, mx, nil
}

// AliasPlan is the address an alias will have, worked out before it is made.
//
// Proton decides the tail: `aliases options` offers suffixes it invents on the
// spot, each signed, and whichever gets used must be sent back with the
// signature it came with. So the address is knowable in advance, but only by
// asking - which is also what makes a suffix nobody offered a refusal that
// arrives before anything exists.
type AliasPlan struct {
	// Address is what mail sent to the alias will be addressed to.
	Address string

	prefix  string
	signed  string
	mailbox int
}

// PlanAlias works out the address, the suffix to sign it with, and the mailbox it
// forwards to, without making anything.
func (s *Service) PlanAlias(ctx context.Context, shareID, prefix, suffix, mailbox string) (*AliasPlan, error) {
	suffixes, mailboxes, err := s.AliasOptions(ctx, shareID)
	if err != nil {
		return nil, err
	}
	chosen, err := pickSuffix(suffixes, suffix)
	if err != nil {
		return nil, err
	}
	mbox, err := pickMailbox(mailboxes, mailbox)
	if err != nil {
		return nil, err
	}
	return &AliasPlan{
		Address: prefix + chosen.Suffix,
		prefix:  prefix, signed: chosen.SignedSuffix, mailbox: mbox,
	}, nil
}

// AliasCreate makes the alias the plan describes and returns the new item's ID.
func (s *Service) AliasCreate(ctx context.Context, u *keys.Unlocked, shareID string, plan *AliasPlan, name string) (string, error) {
	sk, err := s.decryptShareKeys(ctx, shareID, u)
	if err != nil {
		return "", err
	}
	shareKey, rotation := sk.latest()
	item := &pb.Item{Metadata: &pb.Metadata{Name: name}, Content: &pb.Content{Content: &pb.Content_Alias{Alias: &pb.ItemAlias{}}}}
	itemKey, err := aead.NewKey()
	if err != nil {
		return "", err
	}
	pbBytes, _ := proto.Marshal(item)
	ct, err := aead.Encrypt(itemKey, pbBytes, []byte(aead.TagItemContent))
	if err != nil {
		return "", err
	}
	ek, err := aead.Encrypt(shareKey, itemKey, []byte(aead.TagItemKey))
	if err != nil {
		return "", err
	}
	var r struct{ Item struct{ ItemID string } }
	if err := s.C.Decode(ctx, proton.Request{
		Method: "POST", Path: "/pass/v1/share/" + shareID + "/alias/custom",
		Body: map[string]any{
			"Prefix":       plan.prefix,
			"SignedSuffix": plan.signed,
			"MailboxIDs":   []int{plan.mailbox},
			"Item": map[string]any{
				"Content":              base64.StdEncoding.EncodeToString(ct),
				"ContentFormatVersion": 7,
				"ItemKey":              base64.StdEncoding.EncodeToString(ek),
				"KeyRotation":          rotation,
			},
		},
	}, &r); err != nil {
		return "", err
	}
	return r.Item.ItemID, nil
}

func pickSuffix(s []AliasSuffix, wanted string) (AliasSuffix, error) {
	if wanted == "" {
		if len(s) == 0 {
			return AliasSuffix{}, fmt.Errorf("no alias suffixes available")
		}
		return s[0], nil
	}
	for _, x := range s {
		if x.Suffix == wanted || strings.HasSuffix(x.Suffix, wanted) {
			return x, nil
		}
	}
	avail := make([]string, 0, len(s))
	for _, x := range s {
		avail = append(avail, x.Suffix)
	}
	return AliasSuffix{}, fmt.Errorf("suffix %q not found; available: %s", wanted, strings.Join(avail, ", "))
}

func pickMailbox(m []AliasMailbox, wanted string) (int, error) {
	if wanted == "" {
		if len(m) == 0 {
			return 0, fmt.Errorf("no mailboxes available")
		}
		return m[0].ID, nil
	}
	for _, x := range m {
		if x.Email == wanted || strings.Contains(x.Email, wanted) {
			return x.ID, nil
		}
	}
	avail := make([]string, 0, len(m))
	for _, x := range m {
		avail = append(avail, x.Email)
	}
	return 0, fmt.Errorf("mailbox %q not found; available: %s", wanted, strings.Join(avail, ", "))
}
