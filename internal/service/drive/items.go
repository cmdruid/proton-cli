package drive

import (
	"context"
	"fmt"
	"strings"

	pgp "github.com/ProtonMail/gopenpgp/v2/crypto"
	"github.com/roman-16/proton-cli/internal/errs"
	"github.com/roman-16/proton-cli/internal/proton"
)

// The kinds a link can be. A response says what a thing is rather than which
// number Proton files it under.
const (
	TypeFolder = "folder"
	TypeFile   = "file"
)

// linkType names Proton's numeric link kind.
func linkType(t int) string {
	if t == protonFolder {
		return TypeFolder
	}
	return TypeFile
}

// protonFolder is Proton's number for a folder link.
const protonFolder = 1

type Child struct {
	LinkID     string `json:"link_id"`
	Name       string `json:"name"`
	Path       string `json:"path,omitempty"`
	Type       string `json:"type"`
	Size       int64  `json:"size"`
	CreateTime int64  `json:"create_time,omitempty"`
	ModifyTime int64  `json:"modify_time,omitempty"`
}

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
		out = append(out, Child{LinkID: r.LinkID, Name: name, Type: linkType(r.Type), Size: r.Size, CreateTime: r.CreateTime, ModifyTime: r.ModifyTime})
	}
	return out, nil
}

// Walk lists all descendants depth-first; each Child carries its full
// decrypted Path.
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
		out = append(out, Child{LinkID: r.LinkID, Name: name, Path: full, Type: linkType(r.Type), Size: r.Size, CreateTime: r.CreateTime, ModifyTime: r.ModifyTime})
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

// CreateFolder requires the parent path to already exist.
func (s *Service) CreateFolder(ctx context.Context, dc *Context, fullPath string) error {
	parent := dirOf(fullPath)
	name := baseOf(fullPath)

	p, err := s.ResolvePath(ctx, dc, parent)
	if err != nil {
		return fmt.Errorf("parent not found: %w", err)
	}
	hashKey, err := hashKeyOf(p.Link, p.NodeKR)
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
	err = s.C.Decode(ctx, proton.Request{Method: "POST", Path: "/drive/shares/" + p.ShareID + "/folders", Body: body}, nil)
	if proton.AlreadyExists(err) {
		return &errs.Exists{Kind: "folder", Name: name, Where: parent}
	}
	return err
}

func (s *Service) Rename(ctx context.Context, dc *Context, path, newName string) error {
	res, err := s.ResolvePath(ctx, dc, path)
	if err != nil {
		return err
	}
	parentLink, err := s.getLink(ctx, res.ShareID, res.Link.ParentLinkID)
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
	hk, err := hashKeyOf(dst.Link, dst.NodeKR)
	if err != nil {
		return err
	}
	newHash, err := lookupHash(strings.ToLower(src.Name), hk)
	if err != nil {
		return err
	}
	encName, err := reEncryptName(src.Link.Name, src.Name, src.ParentKR, dst.NodeKR, dc.AddrKR)
	if err != nil {
		return err
	}
	newPass, _, err := reEncryptNodePassphrase(src.Link, src.ParentKR, dst.NodeKR, dc.AddrKR)
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

// Copy duplicates a file into destPath. The node passphrase and name are
// re-encrypted to the destination folder's node key (the content is copied
// server-side); the source is left in place.
func (s *Service) Copy(ctx context.Context, dc *Context, sourcePath, destPath string) error {
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
	hk, err := hashKeyOf(dst.Link, dst.NodeKR)
	if err != nil {
		return err
	}
	newHash, err := lookupHash(strings.ToLower(src.Name), hk)
	if err != nil {
		return err
	}
	encName, err := reEncryptName(src.Link.Name, src.Name, src.ParentKR, dst.NodeKR, dc.AddrKR)
	if err != nil {
		return err
	}
	newPass, _, err := reEncryptNodePassphrase(src.Link, src.ParentKR, dst.NodeKR, dc.AddrKR)
	if err != nil {
		return fmt.Errorf("re-encrypt passphrase: %w", err)
	}
	return s.C.Decode(ctx, proton.Request{
		Method: "POST", Path: fmt.Sprintf("/drive/volumes/%s/links/%s/copy", dc.VolumeID, src.LinkID),
		Body: map[string]any{
			"Name": encName, "Hash": newHash, "NodePassphrase": newPass,
			"TargetVolumeID": dc.VolumeID, "TargetParentLinkID": dst.LinkID,
			"NameSignatureEmail": dc.AddrEmail,
		},
	}, nil)
}

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
