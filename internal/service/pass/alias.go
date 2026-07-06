package pass

import (
	"context"
	"encoding/base64"
	"fmt"
	"strings"

	"github.com/roman-16/proton-cli/internal/account/keys"
	"github.com/roman-16/proton-cli/internal/crypto/aead"
	pb "github.com/roman-16/proton-cli/internal/proto"
	"github.com/roman-16/proton-cli/internal/proton"
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

func (s *Service) AliasCreate(ctx context.Context, u *keys.Unlocked, shareID, prefix, suffix, mailbox, name string) (string, error) {
	suffixes, mailboxes, err := s.AliasOptions(ctx, shareID)
	if err != nil {
		return "", err
	}
	signed, err := pickSuffix(suffixes, suffix)
	if err != nil {
		return "", err
	}
	mbox, err := pickMailbox(mailboxes, mailbox)
	if err != nil {
		return "", err
	}
	sk, err := s.decryptShareKeys(ctx, shareID, u)
	if err != nil {
		return "", err
	}
	shareKey, rotation := sk.latest()
	if name == "" {
		name = prefix
	}
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
			"Prefix":       prefix,
			"SignedSuffix": signed,
			"MailboxIDs":   []int{mbox},
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

func pickSuffix(s []AliasSuffix, wanted string) (string, error) {
	if wanted == "" {
		if len(s) == 0 {
			return "", fmt.Errorf("no alias suffixes available")
		}
		return s[0].SignedSuffix, nil
	}
	for _, x := range s {
		if x.Suffix == wanted || strings.HasSuffix(x.Suffix, wanted) {
			return x.SignedSuffix, nil
		}
	}
	avail := make([]string, 0, len(s))
	for _, x := range s {
		avail = append(avail, x.Suffix)
	}
	return "", fmt.Errorf("suffix %q not found; available: %s", wanted, strings.Join(avail, ", "))
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
