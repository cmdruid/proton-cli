package drive

import (
	"context"
	"fmt"
	"strings"

	pgp "github.com/ProtonMail/gopenpgp/v2/crypto"
	"github.com/roman-16/proton-cli/internal/proton"
)

// Child describes an item in a folder listing.
type Child struct {
	LinkID     string `json:"link_id"`
	Name       string `json:"name"`
	Path       string `json:"path,omitempty"`
	Type       int    `json:"type"`
	Size       int64  `json:"size"`
	CreateTime int64  `json:"create_time,omitempty"`
	ModifyTime int64  `json:"modify_time,omitempty"`
}

// List returns decrypted child entries of a folder.
func (s *Service) List(ctx context.Context, dc *Context, path string) ([]Child, error) {
	res, err := s.ResolvePath(ctx, dc, path)
	if err != nil {
		return nil, err
	}
	if !res.IsFolder {
		return nil, fmt.Errorf("%s is not a folder", path)
	}
	raw, err := s.listRawChildren(ctx, res.ShareID, res.LinkID)
	if err != nil {
		return nil, err
	}
	out := make([]Child, 0, len(raw))
	for _, r := range raw {
		name, err := decryptName(r.Name, res.NodeKR)
		if err != nil {
			name = "(decrypt failed)"
		}
		out = append(out, Child{LinkID: r.LinkID, Name: name, Type: r.Type, Size: r.Size, CreateTime: r.CreateTime, ModifyTime: r.ModifyTime})
	}
	return out, nil
}

// Walk recursively lists all descendants of path (depth-first, folders before
// contents), with full decrypted paths in Path.
func (s *Service) Walk(ctx context.Context, dc *Context, path string) ([]Child, error) {
	res, err := s.ResolvePath(ctx, dc, path)
	if err != nil {
		return nil, err
	}
	if !res.IsFolder {
		return nil, fmt.Errorf("%s is not a folder", path)
	}
	return s.walk(ctx, res.ShareID, res.LinkID, res.NodeKR, strings.TrimRight(path, "/"))
}

func (s *Service) walk(ctx context.Context, shareID, linkID string, parentKR *pgp.KeyRing, prefix string) ([]Child, error) {
	raw, err := s.listRawChildren(ctx, shareID, linkID)
	if err != nil {
		return nil, err
	}
	var out []Child
	for _, r := range raw {
		name, err := decryptName(r.Name, parentKR)
		if err != nil {
			name = "(decrypt failed)"
		}
		full := prefix + "/" + name
		out = append(out, Child{LinkID: r.LinkID, Name: name, Path: full, Type: r.Type, Size: r.Size, CreateTime: r.CreateTime, ModifyTime: r.ModifyTime})
		if r.Type == 1 {
			childKR, err := unlockNode(&r, parentKR, nil)
			if err != nil {
				continue
			}
			nested, err := s.walk(ctx, shareID, r.LinkID, childKR, full)
			if err != nil {
				continue
			}
			out = append(out, nested...)
		}
	}
	return out, nil
}

// CreateFolder creates a new folder at the given path (parent must exist).
func (s *Service) CreateFolder(ctx context.Context, dc *Context, fullPath string) error {
	parent := dirOf(fullPath)
	name := baseOf(fullPath)

	p, err := s.ResolvePath(ctx, dc, parent)
	if err != nil {
		return fmt.Errorf("parent not found: %w", err)
	}
	parentLink, err := s.getLink(ctx, p.ShareID, p.LinkID)
	if err != nil {
		return err
	}
	hashKey, err := hashKeyOf(parentLink, p.NodeKR)
	if err != nil {
		return err
	}
	hash, err := lookupHash(strings.ToLower(name), hashKey)
	if err != nil {
		return err
	}
	encName, err := encryptName(name, p.NodeKR, dc.AddrKR)
	if err != nil {
		return err
	}
	nodeKey, nodePass, nodePassSig, nodePriv, err := genNodeKeys(p.NodeKR, dc.AddrKR)
	if err != nil {
		return err
	}
	nodeKR, err := pgp.NewKeyRing(nodePriv)
	if err != nil {
		return err
	}
	hashKeyEnc, err := genNodeHashKey(nodeKR, nodeKR)
	if err != nil {
		return err
	}
	body := map[string]any{
		"Name":                    encName,
		"Hash":                    hash,
		"ParentLinkID":            p.LinkID,
		"NodePassphrase":          nodePass,
		"NodePassphraseSignature": nodePassSig,
		"SignatureAddress":        dc.AddrEmail,
		"NodeKey":                 nodeKey,
		"NodeHashKey":             hashKeyEnc,
	}
	return s.C.Decode(ctx, proton.Request{Method: "POST", Path: "/drive/shares/" + p.ShareID + "/folders", Body: body}, nil)
}

// Rename renames a file or folder in place.
func (s *Service) Rename(ctx context.Context, dc *Context, path, newName string) error {
	res, err := s.ResolvePath(ctx, dc, path)
	if err != nil {
		return err
	}
	resLink, err := s.getLink(ctx, res.ShareID, res.LinkID)
	if err != nil {
		return err
	}
	parentLink, err := s.getLink(ctx, res.ShareID, resLink.ParentLinkID)
	if err != nil {
		return err
	}
	hk, err := hashKeyOf(parentLink, res.ParentKR)
	if err != nil {
		return err
	}
	newHash, err := lookupHash(strings.ToLower(newName), hk)
	if err != nil {
		return err
	}
	oldHash, _ := lookupHash(strings.ToLower(res.Name), hk)
	encName, err := encryptName(newName, res.ParentKR, dc.AddrKR)
	if err != nil {
		return err
	}
	return s.C.Decode(ctx, proton.Request{
		Method: "PUT", Path: fmt.Sprintf("/drive/shares/%s/links/%s/rename", res.ShareID, res.LinkID),
		Body: map[string]any{"Name": encName, "Hash": newHash, "OriginalHash": oldHash, "NameSignatureEmail": dc.AddrEmail},
	}, nil)
}

// Move relocates a file/folder to a different parent folder.
func (s *Service) Move(ctx context.Context, dc *Context, sourcePath, destPath string) error {
	src, err := s.ResolvePath(ctx, dc, sourcePath)
	if err != nil {
		return fmt.Errorf("source not found: %w", err)
	}
	dst, err := s.ResolvePath(ctx, dc, destPath)
	if err != nil {
		return fmt.Errorf("destination not found: %w", err)
	}
	if !dst.IsFolder {
		return fmt.Errorf("%s is not a folder", destPath)
	}
	srcLink, err := s.getLink(ctx, src.ShareID, src.LinkID)
	if err != nil {
		return err
	}
	dstLink, err := s.getLink(ctx, dst.ShareID, dst.LinkID)
	if err != nil {
		return err
	}
	hk, err := hashKeyOf(dstLink, dst.NodeKR)
	if err != nil {
		return err
	}
	newHash, err := lookupHash(strings.ToLower(src.Name), hk)
	if err != nil {
		return err
	}
	encName, err := reEncryptName(srcLink.Name, src.Name, src.ParentKR, dst.NodeKR, dc.AddrKR)
	if err != nil {
		return err
	}
	newPass, _, err := reEncryptNodePassphrase(srcLink, src.ParentKR, dst.NodeKR, dc.AddrKR)
	if err != nil {
		return fmt.Errorf("re-encrypt passphrase: %w", err)
	}
	return s.C.Decode(ctx, proton.Request{
		Method: "PUT", Path: fmt.Sprintf("/drive/shares/%s/links/%s/move", src.ShareID, src.LinkID),
		Body: map[string]any{
			"Name": encName, "Hash": newHash, "ParentLinkID": dst.LinkID,
			"NodePassphrase": newPass, "NameSignatureEmail": dc.AddrEmail,
		},
	}, nil)
}

// Delete moves an item to trash (or permanently deletes when permanent=true).
func (s *Service) Delete(ctx context.Context, dc *Context, path string, permanent bool) error {
	res, err := s.ResolvePath(ctx, dc, path)
	if err != nil {
		return err
	}
	if err := s.C.Decode(ctx, proton.Request{
		Method: "POST", Path: "/drive/v2/volumes/" + dc.VolumeID + "/trash_multiple",
		Body: map[string]any{"LinkIDs": []string{res.LinkID}},
	}, nil); err != nil {
		return err
	}
	if permanent {
		return s.C.Decode(ctx, proton.Request{
			Method: "POST", Path: "/drive/v2/volumes/" + dc.VolumeID + "/trash/delete_multiple",
			Body: map[string]any{"LinkIDs": []string{res.LinkID}},
		}, nil)
	}
	return nil
}
