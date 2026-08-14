package pass

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"

	pgp "github.com/ProtonMail/gopenpgp/v2/crypto"
	"github.com/roman-16/proton-cli/internal/crypto/aead"
	"github.com/roman-16/proton-cli/internal/errs"
	"github.com/roman-16/proton-cli/internal/fetch"
	"github.com/roman-16/proton-cli/internal/proton"
	pb "github.com/roman-16/proton-cli/internal/service/pass/proto"
	"google.golang.org/protobuf/proto"
)

type Vault struct {
	ShareID     string `json:"share_id"`
	VaultID     string `json:"vault_id"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Owner       bool   `json:"owner"`
	Shared      bool   `json:"shared"`
	Members     int    `json:"members"`
	AddressID   string `json:"address_id,omitempty"`
}

func (s *Service) VaultsList(ctx context.Context) ([]Vault, error) {
	// Every vault in the answer will be opened with the account's keys, so they
	// are asked for while the list is on its way.
	var raw []json.RawMessage
	if _, err := s.keys.Alongside(ctx, func(ctx context.Context) error {
		var err error
		raw, err = s.getShares(ctx)
		return err
	}); err != nil {
		return nil, err
	}
	type share struct {
		ShareID            string
		VaultID            string
		TargetType         int
		Owner              bool
		Shared             bool
		TargetMembers      int
		AddressID          string
		Content            string
		ContentKeyRotation int
	}
	var shares []share
	for _, r := range raw {
		var sh share
		if err := json.Unmarshal(r, &sh); err != nil {
			continue
		}
		if sh.TargetType != 1 {
			continue
		}
		shares = append(shares, sh)
	}

	// Each vault's name is encrypted with its own share's key, so every key is
	// asked for at the same time rather than one vault at a time. A share whose
	// keys cannot be read is reported without a name, as it was before, and the
	// answer is remembered so the second look costs nothing.
	fetches := make([]func(context.Context) error, 0, len(shares))
	for _, sh := range shares {
		if sh.Content == "" {
			continue
		}
		fetches = append(fetches, func(ctx context.Context) error {
			_, _ = s.decryptShareKeys(ctx, sh.ShareID)
			return nil
		})
	}
	_ = fetch.Together(ctx, fetches...)

	var out []Vault
	for _, sh := range shares {
		v := Vault{
			ShareID: sh.ShareID, VaultID: sh.VaultID,
			Owner: sh.Owner, Shared: sh.Shared,
			Members: sh.TargetMembers, AddressID: sh.AddressID,
		}
		if sh.Content != "" {
			sk, err := s.decryptShareKeys(ctx, sh.ShareID)
			if err == nil {
				if key, ok := sk.keys[sh.ContentKeyRotation]; ok {
					if vv, err := decryptVault(sh.Content, key); err == nil {
						v.Name = vv.Name
						v.Description = vv.Description
					}
				}
			}
		}
		out = append(out, v)
	}
	return out, nil
}

func (s *Service) VaultCreate(ctx context.Context, name string) (string, error) {
	u, err := s.keys(ctx)
	if err != nil {
		return "", err
	}
	vault := &pb.Vault{Name: name}
	rawKey, err := aead.NewKey()
	if err != nil {
		return "", err
	}
	msg := pgp.NewPlainMessage(rawKey)
	encKey, err := u.UserKR.Encrypt(msg, u.UserKR)
	if err != nil {
		return "", err
	}
	encVaultKey := base64.StdEncoding.EncodeToString(encKey.GetBinary())
	pbBytes, err := proto.Marshal(vault)
	if err != nil {
		return "", err
	}
	ct, err := aead.Encrypt(rawKey, pbBytes, []byte(aead.TagVaultContent))
	if err != nil {
		return "", err
	}
	_, addr, err := u.PrimaryAddr()
	if err != nil {
		return "", err
	}
	var r struct{ Share struct{ ShareID string } }
	if err := s.C.Decode(ctx, proton.Request{
		Method: "POST", Path: "/pass/v1/vault",
		Body: map[string]any{
			"AddressID":            addr.ID,
			"ContentFormatVersion": 1,
			"Content":              base64.StdEncoding.EncodeToString(ct),
			"EncryptedVaultKey":    encVaultKey,
		},
	}, &r); err != nil {
		return "", err
	}
	return r.Share.ShareID, nil
}

func (s *Service) VaultDelete(ctx context.Context, shareID string) error {
	return s.C.Decode(ctx, proton.Request{Method: "DELETE", Path: "/pass/v1/vault/" + shareID}, nil)
}

// VaultEdit renames a vault, preserving its description and display settings by
// re-encrypting the existing vault content with the latest share key.
func (s *Service) VaultEdit(ctx context.Context, shareID, newName string) error {
	shares, err := s.getShares(ctx)
	if err != nil {
		return err
	}
	var content string
	var rotation int
	found := false
	for _, raw := range shares {
		var sh struct {
			ShareID            string
			TargetType         int
			Content            string
			ContentKeyRotation int
		}
		if err := json.Unmarshal(raw, &sh); err != nil {
			continue
		}
		if sh.ShareID == shareID && sh.TargetType == 1 {
			content, rotation, found = sh.Content, sh.ContentKeyRotation, true
			break
		}
	}
	if !found {
		return &errs.NotFound{Kind: "vault", Ref: shareID}
	}
	sk, err := s.decryptShareKeys(ctx, shareID)
	if err != nil {
		return err
	}
	shareKey, ok := sk.keys[rotation]
	if !ok {
		return fmt.Errorf("no share key for rotation %d", rotation)
	}
	pbBytes, err := renamedVault(content, shareKey, newName)
	if err != nil {
		return fmt.Errorf("rename vault %s: %w", shareID, err)
	}
	writeKey, writeRotation := sk.latest()
	if writeKey == nil {
		return fmt.Errorf("vault %s has no usable share key", shareID)
	}
	ct, err := aead.Encrypt(writeKey, pbBytes, []byte(aead.TagVaultContent))
	if err != nil {
		return err
	}
	return s.C.Decode(ctx, proton.Request{
		Method: "PUT", Path: "/pass/v1/vault/" + shareID,
		Body: map[string]any{
			"Content":              base64.StdEncoding.EncodeToString(ct),
			"ContentFormatVersion": 1,
			"KeyRotation":          writeRotation,
		},
	}, nil)
}

func (s *Service) ResolveVault(ctx context.Context, nameOrID string) (string, error) {
	vaults, err := s.VaultsList(ctx)
	if err != nil {
		return "", err
	}
	if nameOrID == "" {
		if len(vaults) == 0 {
			return "", &errs.NotFound{Kind: "vault"}
		}
		return vaults[0].ShareID, nil
	}
	for _, v := range vaults {
		if v.ShareID == nameOrID {
			return v.ShareID, nil
		}
	}
	for _, v := range vaults {
		if v.Name == nameOrID {
			return v.ShareID, nil
		}
	}
	return "", &errs.NotFound{Kind: "vault", Ref: nameOrID}
}

// renamedVault is the stored vault with a new name, and nothing else changed.
//
// It fails rather than substituting an empty vault when the existing content
// cannot be read: a rename that rebuilds the vault from nothing is a way of losing
// its description and every other field it holds.
func renamedVault(content string, shareKey []byte, newName string) ([]byte, error) {
	if content == "" {
		return nil, fmt.Errorf("it has no stored content to rename")
	}
	vault, err := decryptVault(content, shareKey)
	if err != nil {
		return nil, fmt.Errorf("its stored content could not be read: %w", err)
	}
	vault.Name = newName
	return proto.Marshal(vault)
}

func decryptVault(encContent string, shareKey []byte) (*pb.Vault, error) {
	data, err := base64.StdEncoding.DecodeString(encContent)
	if err != nil {
		return nil, err
	}
	plain, err := aead.Decrypt(shareKey, data, []byte(aead.TagVaultContent))
	if err != nil {
		return nil, err
	}
	var v pb.Vault
	if err := proto.Unmarshal(plain, &v); err != nil {
		return nil, err
	}
	return &v, nil
}
