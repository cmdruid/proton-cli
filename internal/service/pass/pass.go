package pass

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"

	pgp "github.com/ProtonMail/gopenpgp/v2/crypto"
	"github.com/roman-16/proton-cli/internal/account/keys"
	"github.com/roman-16/proton-cli/internal/fetch"
	"github.com/roman-16/proton-cli/internal/proton"
)

type Service struct {
	C proton.Doer
	// A share's keys are wanted twice by most commands - once to read the vault's
	// name and again to read what is in it - and once per vault by any command that
	// covers them all.
	shareKeys fetch.Memo[*shareKeys]
}

func New(c proton.Doer) *Service { return &Service{C: c} }

type shareKeys struct{ keys map[int][]byte }

func (sk *shareKeys) latest() ([]byte, int) {
	max := -1
	for r := range sk.keys {
		if r > max {
			max = r
		}
	}
	return sk.keys[max], max
}

func (s *Service) getShares(ctx context.Context) ([]json.RawMessage, error) {
	var r struct{ Shares []json.RawMessage }
	if err := s.C.Decode(ctx, proton.Request{Method: "GET", Path: "/pass/v1/share"}, &r); err != nil {
		return nil, err
	}
	return r.Shares, nil
}

func (s *Service) decryptShareKeys(ctx context.Context, shareID string, u *keys.Unlocked) (*shareKeys, error) {
	return s.shareKeys.Do(shareID, func() (*shareKeys, error) {
		var r struct {
			ShareKeys struct {
				Keys []json.RawMessage
			}
		}
		if err := s.C.Decode(ctx, proton.Request{Method: "GET", Path: "/pass/v1/share/" + shareID + "/key", Query: proton.Query("Page", "0")}, &r); err != nil {
			return nil, err
		}
		out := &shareKeys{keys: map[int][]byte{}}
		for _, raw := range r.ShareKeys.Keys {
			var k struct {
				Key         string
				KeyRotation int
			}
			if err := json.Unmarshal(raw, &k); err != nil {
				continue
			}
			kb, err := base64.StdEncoding.DecodeString(k.Key)
			if err != nil {
				continue
			}
			msg := pgp.NewPGPMessage(kb)
			dec, err := u.UserKR.Decrypt(msg, u.UserKR, pgp.GetUnixTime())
			if err != nil {
				continue
			}
			out.keys[k.KeyRotation] = dec.GetBinary()
		}
		if len(out.keys) == 0 {
			return nil, fmt.Errorf("failed to decrypt share keys for %s", shareID)
		}
		return out, nil
	})
}
